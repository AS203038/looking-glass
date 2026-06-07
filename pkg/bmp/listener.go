package bmp

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"

	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/redis/go-redis/v9"
)

// StartBMPListener initializes the concurrent TCP server to receive BMP feeds.
func StartBMPListener(ctx context.Context, listenAddr string, rdb *redis.Client, rm utils.RouterMap) error {
	log := logging.Component("bmp")
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Error("failed to start BMP listener", slog.String("listen", listenAddr), slog.Any("err", err))
		return err
	}
	log.Info("stateless BMP listener active", slog.String("listen", listenAddr))

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	go func() {
		defer ln.Close()
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Debug("BMP accept failed", slog.Any("err", err))
					continue
				}
			}
			go handleBMPConnection(conn, rdb, rm)
		}
	}()

	return nil
}

// handleBMPConnection consumes BMP messages on a single session.
func handleBMPConnection(conn net.Conn, rdb *redis.Client, rm utils.RouterMap) {
	defer conn.Close()

	routerName := findRouterByRemoteIP(conn.RemoteAddr(), rm)
	if routerName == "" {
		host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		routerName = host
	}

	log := logging.Component("bmp").With(
		slog.String("router", routerName),
		slog.String("remote", conn.RemoteAddr().String()))
	log.Info("BMP router telemetry feed connected")

	headerBuf := make([]byte, bmpCommonHeaderLen)
	for {
		_, err := io.ReadFull(conn, headerBuf)
		if err != nil {
			log.Info("BMP telemetry feed disconnected", slog.Any("err", err))
			break
		}
		version, length, msgType, err := ParseBMPCommonHeader(headerBuf)
		if err != nil {
			log.Error("malformed BMP common header", slog.Any("err", err))
			break
		}
		if version != 3 {
			log.Error("unsupported BMP version", slog.Int("version", int(version)))
			break
		}
		if length < bmpCommonHeaderLen {
			log.Error("invalid BMP message length", slog.Any("length", length))
			break
		}

		payloadLen := length - bmpCommonHeaderLen
		payload := make([]byte, payloadLen)
		_, err = io.ReadFull(conn, payload)
		if err != nil {
			log.Error("failed to read BMP payload", slog.Any("err", err))
			break
		}

		switch msgType {
		case BMPMsgTypePeerUp:
			if len(payload) < bmpPerPeerHeaderLen {
				continue
			}
			peer, err := ParsePerPeerHeader(payload[:bmpPerPeerHeaderLen])
			if err != nil {
				continue
			}
			log.Info("BMP BGP Peer session established",
				slog.String("peer_ip", peer.PeerAddress),
				slog.Int("peer_asn", int(peer.PeerAS)))
			_ = UpdateBMPPeerStatus(context.Background(), rdb, routerName, peer.PeerAddress, peer.PeerAS, "established", peer.PeerBGPID)

		case BMPMsgTypePeerDown:
			if len(payload) < bmpPerPeerHeaderLen {
				continue
			}
			peer, err := ParsePerPeerHeader(payload[:bmpPerPeerHeaderLen])
			if err != nil {
				continue
			}
			log.Info("BMP BGP Peer session terminated",
				slog.String("peer_ip", peer.PeerAddress),
				slog.Int("peer_asn", int(peer.PeerAS)))
			_ = UpdateBMPPeerStatus(context.Background(), rdb, routerName, peer.PeerAddress, peer.PeerAS, "down", peer.PeerBGPID)

		case BMPMsgTypeRouteMonitoring:
			if len(payload) < bmpPerPeerHeaderLen {
				continue
			}
			peer, err := ParsePerPeerHeader(payload[:bmpPerPeerHeaderLen])
			if err != nil {
				continue
			}
			bgpPayload := payload[bmpPerPeerHeaderLen:]
			if len(bgpPayload) < 19 {
				continue
			}
			bgpLength := binary.BigEndian.Uint16(bgpPayload[16:18])
			bgpType := bgpPayload[18]

			if bgpType == 2 && len(bgpPayload) >= int(bgpLength) && bgpLength > 19 {
				update, err := ParseBGPUpdate(bgpPayload[19:bgpLength])
				if err == nil {
					_ = UpdateBMPRib(context.Background(), rdb, routerName, peer.PeerAddress, update)
				}
			}
		}
	}
}

// findRouterByRemoteIP matches the incoming BMP connection IP with configured devices.
func findRouterByRemoteIP(remoteAddr net.Addr, rm utils.RouterMap) string {
	host, _, _ := net.SplitHostPort(remoteAddr.String())
	for _, dev := range rm {
		if dev.Config == nil {
			continue
		}
		cfgHost := dev.Config.Hostname
		if h, _, err := net.SplitHostPort(cfgHost); err == nil {
			cfgHost = h
		}
		if cfgHost == host {
			return dev.Config.Name
		}
		addrs, err := net.LookupHost(cfgHost)
		if err == nil {
			for _, a := range addrs {
				if a == host {
					return dev.Config.Name
				}
			}
		}
	}
	return ""
}
