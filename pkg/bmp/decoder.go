package bmp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

// bmpCommonHeaderLen is the length of the Common Header (RFC 7854).
const bmpCommonHeaderLen = 6

// bmpPerPeerHeaderLen is the length of the Per-Peer Header.
const bmpPerPeerHeaderLen = 42

const (
	BMPMsgTypeRouteMonitoring = 0
	BMPMsgTypeStatsReport     = 1
	BMPMsgTypePeerDown        = 2
	BMPMsgTypePeerUp          = 3
	BMPMsgTypeInitiation      = 4
	BMPMsgTypeTermination     = 5
)

// BMPMessage represents a parsed BMP message.
type BMPMessage struct {
	Version uint8
	Length  uint32
	Type    uint8
	Peer    *PerPeerHeader
	Payload []byte
}

// PerPeerHeader represents the metadata about a BGP peer.
type PerPeerHeader struct {
	PeerType          uint8
	Flags             uint8
	PeerDistinguisher [8]byte
	PeerAddress       string
	PeerAS            uint32
	PeerBGPID         string
	TimestampSec      uint32
	TimestampMicro    uint32
}

// IsIPv6 reports whether the BGP peer is IPv6.
func (h *PerPeerHeader) IsIPv6() bool {
	return (h.Flags & 0x80) != 0
}

// ParseBMPCommonHeader parses the first 6 bytes of a BMP message.
func ParseBMPCommonHeader(data []byte) (uint8, uint32, uint8, error) {
	if len(data) < bmpCommonHeaderLen {
		return 0, 0, 0, errors.New("truncated BMP common header")
	}
	version := data[0]
	length := binary.BigEndian.Uint32(data[1:5])
	msgType := data[5]
	return version, length, msgType, nil
}

// ParsePerPeerHeader parses the 42-byte Per-Peer Header.
func ParsePerPeerHeader(data []byte) (*PerPeerHeader, error) {
	if len(data) < bmpPerPeerHeaderLen {
		return nil, errors.New("truncated Per-Peer Header")
	}
	h := &PerPeerHeader{}
	h.PeerType = data[0]
	h.Flags = data[1]
	copy(h.PeerDistinguisher[:], data[2:10])

	addrBytes := data[10:26]
	if h.IsIPv6() {
		h.PeerAddress = net.IP(addrBytes).String()
	} else {
		h.PeerAddress = net.IP(addrBytes[12:16]).String()
	}

	h.PeerAS = binary.BigEndian.Uint32(data[26:30])
	h.PeerBGPID = net.IP(data[30:34]).String()
	h.TimestampSec = binary.BigEndian.Uint32(data[34:38])
	h.TimestampMicro = binary.BigEndian.Uint32(data[38:42])

	return h, nil
}

// BGPUpdate holds the parsed elements of a BGP Update message.
type BGPUpdate struct {
	AnnouncedPrefixes []string
	WithdrawnPrefixes []string
	ASPath            []uint32
	NextHop           string
	Communities       []string
	LargeCommunities  []string
}

// ParseBGPUpdate parses a raw BGP Update payload.
func ParseBGPUpdate(data []byte) (*BGPUpdate, error) {
	if len(data) < 2 {
		return nil, errors.New("truncated BGP Update header")
	}
	update := &BGPUpdate{}

	withdrawnLen := int(binary.BigEndian.Uint16(data[0:2]))
	if len(data) < 2+withdrawnLen+2 {
		return nil, errors.New("truncated withdrawn routes block")
	}
	offset := 2
	endWithdrawn := offset + withdrawnLen
	for offset < endWithdrawn {
		prefix, read, err := parsePrefix(data[offset:endWithdrawn], false)
		if err != nil {
			break
		}
		update.WithdrawnPrefixes = append(update.WithdrawnPrefixes, prefix)
		offset += read
	}

	offset = endWithdrawn
	attrsLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if len(data) < offset+attrsLen {
		return nil, errors.New("truncated path attributes block")
	}
	endAttrs := offset + attrsLen
	attrs, err := parsePathAttributes(data[offset:endAttrs])
	if err == nil {
		for _, attr := range attrs {
			switch attr.Type {
			case 2: // AS_PATH
				update.ASPath = parseASPath(attr.Value)
			case 3: // NEXT_HOP
				update.NextHop = parseNextHop(attr.Value)
			case 8: // COMMUNITIES
				update.Communities = parseCommunities(attr.Value)
			case 32: // LARGE_COMMUNITIES
				update.LargeCommunities = parseLargeCommunities(attr.Value)
			case 14: // MP_REACH_NLRI (IPv6 NLRI + Next-Hop)
				nextHop, prefixes, err := parseMPReachNLRI(attr.Value)
				if err == nil {
					if nextHop != "" {
						update.NextHop = nextHop
					}
					update.AnnouncedPrefixes = append(update.AnnouncedPrefixes, prefixes...)
				}
			case 15: // MP_UNREACH_NLRI (IPv6 Withdrawals)
				prefixes, err := parseMPUnreachNLRI(attr.Value)
				if err == nil {
					update.WithdrawnPrefixes = append(update.WithdrawnPrefixes, prefixes...)
				}
			}
		}
	}

	offset = endAttrs
	for offset < len(data) {
		prefix, read, err := parsePrefix(data[offset:], false)
		if err != nil {
			break
		}
		update.AnnouncedPrefixes = append(update.AnnouncedPrefixes, prefix)
		offset += read
	}

	return update, nil
}

// parsePrefix extracts an IPNet prefix in "IP/Length" format.
func parsePrefix(data []byte, isIPv6 bool) (string, int, error) {
	if len(data) == 0 {
		return "", 0, errors.New("empty prefix")
	}
	length := int(data[0])
	bytesLen := (length + 7) / 8
	if len(data) < 1+bytesLen {
		return "", 0, errors.New("truncated prefix payload")
	}
	ipBytes := make([]byte, 4)
	if isIPv6 {
		ipBytes = make([]byte, 16)
	}
	copy(ipBytes, data[1:1+bytesLen])
	ip := net.IP(ipBytes)
	return fmt.Sprintf("%s/%d", ip.String(), length), 1 + bytesLen, nil
}

// PathAttribute describes one attribute TLV in a BGP update.
type PathAttribute struct {
	Flags uint8
	Type  uint8
	Value []byte
}

// parsePathAttributes extracts attribute TLV structures.
func parsePathAttributes(data []byte) ([]PathAttribute, error) {
	var attrs []PathAttribute
	offset := 0
	for offset < len(data) {
		if len(data)-offset < 2 {
			return nil, errors.New("truncated attribute header")
		}
		flags := data[offset]
		typeCode := data[offset+1]
		offset += 2
		length := 0
		if flags&0x10 != 0 { // Extended length (2 bytes length)
			if len(data)-offset < 2 {
				return nil, errors.New("truncated extended attribute length")
			}
			length = int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2
		} else {
			if len(data)-offset < 1 {
				return nil, errors.New("truncated attribute length")
			}
			length = int(data[offset])
			offset += 1
		}
		if len(data)-offset < length {
			return nil, errors.New("truncated attribute value")
		}
		attrs = append(attrs, PathAttribute{
			Flags: flags,
			Type:  typeCode,
			Value: data[offset : offset+length],
		})
		offset += length
	}
	return attrs, nil
}

// parseASPath decodes the ASNs from an AS_PATH attribute.
func parseASPath(value []byte) []uint32 {
	var asns []uint32
	offset := 0
	for offset < len(value) {
		if len(value)-offset < 2 {
			break
		}
		segType := value[offset]
		segLen := int(value[offset+1])
		offset += 2
		asnSize := 4
		if segLen > 0 && (len(value)-offset)/segLen == 2 {
			asnSize = 2
		}
		for i := 0; i < segLen; i++ {
			if len(value)-offset < asnSize {
				break
			}
			var asn uint32
			if asnSize == 4 {
				asn = binary.BigEndian.Uint32(value[offset : offset+4])
			} else {
				asn = uint32(binary.BigEndian.Uint16(value[offset : offset+2]))
			}
			if segType == 2 { // AS_SEQUENCE
				asns = append(asns, asn)
			}
			offset += asnSize
		}
	}
	return asns
}

// parseCommunities extracts RFC 1997 communities as "ASN:VALUE".
func parseCommunities(value []byte) []string {
	var comms []string
	for i := 0; i < len(value); i += 4 {
		if len(value)-i < 4 {
			break
		}
		asn := binary.BigEndian.Uint16(value[i : i+2])
		val := binary.BigEndian.Uint16(value[i+2 : i+4])
		comms = append(comms, fmt.Sprintf("%d:%d", asn, val))
	}
	return comms
}

// parseLargeCommunities extracts RFC 8092 communities as "Global:Local1:Local2".
func parseLargeCommunities(value []byte) []string {
	var comms []string
	for i := 0; i < len(value); i += 12 {
		if len(value)-i < 12 {
			break
		}
		global := binary.BigEndian.Uint32(value[i : i+4])
		local1 := binary.BigEndian.Uint32(value[i+4 : i+8])
		local2 := binary.BigEndian.Uint32(value[i+8 : i+12])
		comms = append(comms, fmt.Sprintf("%d:%d:%d", global, local1, local2))
	}
	return comms
}

// parseNextHop decodes standard IPv4 next-hop.
func parseNextHop(value []byte) string {
	if len(value) == 4 {
		return net.IP(value).String()
	}
	return ""
}

// parseMPReachNLRI decodes MP_REACH_NLRI (IPv6).
func parseMPReachNLRI(value []byte) (string, []string, error) {
	if len(value) < 5 {
		return "", nil, errors.New("truncated MP_REACH_NLRI")
	}
	afi := binary.BigEndian.Uint16(value[0:2])
	safi := value[2]
	nextHopLen := int(value[3])
	if len(value) < 4+nextHopLen+1 {
		return "", nil, errors.New("truncated MP_REACH_NLRI next hop")
	}
	nextHopIP := ""
	if afi == 2 && safi == 1 { // IPv6 Unicast
		if nextHopLen >= 16 {
			nextHopIP = net.IP(value[4 : 4+16]).String()
		}
	}
	nlriOffset := 4 + nextHopLen + 1
	var prefixes []string
	for nlriOffset < len(value) {
		prefix, read, err := parsePrefix(value[nlriOffset:], afi == 2)
		if err != nil {
			break
		}
		prefixes = append(prefixes, prefix)
		nlriOffset += read
	}
	return nextHopIP, prefixes, nil
}

// parseMPUnreachNLRI decodes MP_UNREACH_NLRI (IPv6).
func parseMPUnreachNLRI(value []byte) ([]string, error) {
	if len(value) < 3 {
		return nil, errors.New("truncated MP_UNREACH_NLRI")
	}
	afi := binary.BigEndian.Uint16(value[0:2])
	offset := 3
	var prefixes []string
	for offset < len(value) {
		prefix, read, err := parsePrefix(value[offset:], afi == 2)
		if err != nil {
			break
		}
		prefixes = append(prefixes, prefix)
		offset += read
	}
	return prefixes, nil
}
