package grpc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/utils"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/ssh"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type mockGrpcRouter struct{}

func (mockRouter mockGrpcRouter) Ping(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return []string{"ping"}, nil
}
func (mockRouter mockGrpcRouter) Traceroute(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return []string{"traceroute"}, nil
}
func (mockRouter mockGrpcRouter) BGPSummary(*utils.RouterConfig) ([]string, error) {
	return []string{"bgp.summary"}, nil
}
func (mockRouter mockGrpcRouter) BGPRoute(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return []string{"bgp.route"}, nil
}
func (mockRouter mockGrpcRouter) BGPCommunity(*utils.RouterConfig, string) ([]string, error) {
	return []string{"bgp.community"}, nil
}
func (mockRouter mockGrpcRouter) BGPLargeCommunity(*utils.RouterConfig, string) ([]string, error) {
	return []string{"bgp.largecommunity"}, nil
}
func (mockRouter mockGrpcRouter) BGPASPath(*utils.RouterConfig, string) ([]string, error) {
	return []string{"bgp.aspath"}, nil
}
func (mockRouter mockGrpcRouter) BGPPeerRoutes(*utils.RouterConfig, string, string, string) ([]string, error) {
	return []string{"bgp.peer_routes"}, nil
}

type mockPoolExhaustedRouter struct {
	mockGrpcRouter
}

func (mockPoolExhaustedRouter) Ping(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return nil, errs.PoolExhausted
}
func (mockPoolExhaustedRouter) Traceroute(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return nil, errs.PoolExhausted
}
func (mockPoolExhaustedRouter) BGPSummary(*utils.RouterConfig) ([]string, error) {
	return nil, errs.PoolExhausted
}
func (mockPoolExhaustedRouter) BGPRoute(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return nil, errs.PoolExhausted
}
func (mockPoolExhaustedRouter) BGPCommunity(*utils.RouterConfig, string) ([]string, error) {
	return nil, errs.PoolExhausted
}
func (mockPoolExhaustedRouter) BGPLargeCommunity(*utils.RouterConfig, string) ([]string, error) {
	return nil, errs.PoolExhausted
}
func (mockPoolExhaustedRouter) BGPASPath(*utils.RouterConfig, string) ([]string, error) {
	return nil, errs.PoolExhausted
}
func (mockPoolExhaustedRouter) BGPPeerRoutes(*utils.RouterConfig, string, string, string) ([]string, error) {
	return nil, errs.PoolExhausted
}

// TestLookingGlassService verifies ConnectRPC services end-to-end.
func TestLookingGlassService(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	config.AddHostKey(signer)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for newChan := range chans {
					if newChan.ChannelType() != "session" {
						newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
						continue
					}
					ch, reqs, err := newChan.Accept()
					if err != nil {
						continue
					}
					go func(channel ssh.Channel, requests <-chan *ssh.Request) {
						defer channel.Close()
						for req := range requests {
							switch req.Type {
							case "exec":
								req.Reply(true, nil)
								channel.Write([]byte("mock output\n"))
								exitStatus := []byte{0, 0, 0, 0}
								channel.SendRequest("exit-status", false, exitStatus)
								return
							default:
								req.Reply(true, nil)
							}
						}
					}(ch, reqs)
				}
			}()
		}
	}()

	redisLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock RESP Redis server: %v", err)
	}
	defer redisLn.Close()

	var cacheMap = make(map[string]string)
	var cacheMu sync.Mutex

	go func() {
		for {
			conn, err := redisLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 2048)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					req := string(buf[:n])
					lowerReq := strings.ToLower(req)

					if strings.Contains(lowerReq, "setinfo") {
						c.Write([]byte("+OK\r\n+OK\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						c.Write([]byte("%1\r\n$5\r\nproto\r\n:2\r\n"))
					} else if strings.Contains(lowerReq, "hlen") {
						if strings.Contains(lowerReq, "test-router") {
							c.Write([]byte(":1\r\n"))
						} else {
							c.Write([]byte(":0\r\n"))
						}
					} else if strings.Contains(lowerReq, "smembers") || strings.Contains(lowerReq, "sinter") {
						c.Write([]byte("*1\r\n$19\r\n10.0.0.2:1.1.1.0/24\r\n"))
					} else if strings.Contains(lowerReq, "zcard") {
						c.Write([]byte(":1\r\n"))
					} else if strings.Contains(lowerReq, "zrange") {
						routeJSON := `{"prefix":"1.1.1.0/24","as_path":[65001,65002,203038],"next_hop":"10.0.0.2","communities":["65000:100"],"large_communities":[],"timestamp":1700000000}`
						c.Write([]byte("*1\r\n$" + strconv.Itoa(len(routeJSON)) + "\r\n" + routeJSON + "\r\n"))
					} else if strings.Contains(lowerReq, "hscan") {
						routeJSON := `{"prefix":"1.1.1.0/24","as_path":[65001,65002,203038],"next_hop":"10.0.0.2","communities":["65000:100"],"large_communities":[],"timestamp":1700000000}`
						c.Write([]byte("*2\r\n$1\r\n0\r\n*2\r\n$10\r\n1.1.1.0/24\r\n$" + strconv.Itoa(len(routeJSON)) + "\r\n" + routeJSON + "\r\n"))
					} else if strings.Contains(lowerReq, "hgetall") {
						if strings.Contains(lowerReq, "lg:bmp:peers:") {
							peerJSON := `{"peer_ip":"10.0.0.2","peer_asn":65001,"state":"established","uptime_seconds":123456,"bgp_id":"10.0.0.2","timestamp":1700000000,"prefixes_received":100,"prefixes_accepted":90}`
							c.Write([]byte("*2\r\n$8\r\n10.0.0.2\r\n$" + strconv.Itoa(len(peerJSON)) + "\r\n" + peerJSON + "\r\n"))
						} else {
							routeJSON := `{"prefix":"1.1.1.0/24","as_path":[65001,65002,203038],"next_hop":"10.0.0.2","communities":["65000:100"],"large_communities":[],"timestamp":1700000000}`
							c.Write([]byte("*2\r\n$10\r\n1.1.1.0/24\r\n$" + strconv.Itoa(len(routeJSON)) + "\r\n" + routeJSON + "\r\n"))
						}
					} else if strings.Contains(lowerReq, "hmget") {
						parts := strings.Split(req, "\r\n")
						count := 0
						if len(parts) > 0 && strings.HasPrefix(parts[0], "*") {
							n, err := strconv.Atoi(parts[0][1:])
							if err == nil {
								count = n - 2
							}
						}
						if count <= 0 {
							count = 1
						}
						var sb strings.Builder
						sb.WriteString("*" + strconv.Itoa(count) + "\r\n")
						routeJSON := `{"prefix":"1.1.1.0/24","as_path":[65001,65002,203038],"next_hop":"10.0.0.2","communities":["65000:100"],"large_communities":[],"timestamp":1700000000}`
						sb.WriteString("$" + strconv.Itoa(len(routeJSON)) + "\r\n" + routeJSON + "\r\n")
						for i := 1; i < count; i++ {
							sb.WriteString("$-1\r\n")
						}
						c.Write([]byte(sb.String()))
					} else if strings.Contains(lowerReq, "get") {
						parts := strings.Split(req, "\r\n")
						key := ""
						for idx, p := range parts {
							if strings.ToLower(p) == "get" && idx+2 < len(parts) {
								key = parts[idx+2]
								break
							}
						}
						cacheMu.Lock()
						val, ok := cacheMap[key]
						cacheMu.Unlock()
						if ok {
							c.Write([]byte("$" + strconv.Itoa(len(val)) + "\r\n" + val + "\r\n"))
						} else {
							c.Write([]byte("$-1\r\n"))
						}
					} else if strings.Contains(lowerReq, "set") {
						parts := strings.Split(req, "\r\n")
						key := ""
						val := ""
						for idx, p := range parts {
							if strings.ToLower(p) == "set" && idx+4 < len(parts) {
								key = parts[idx+2]
								val = parts[idx+4]
								break
							}
						}
						cacheMu.Lock()
						cacheMap[key] = val
						cacheMu.Unlock()
						c.Write([]byte("+OK\r\n"))
					} else {
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	client := redis.NewClient(&redis.Options{
		Addr:     redisLn.Addr().String(),
		Protocol: 2,
	})
	defer client.Close()

	SetRedis(client)
	SetRPCCache(client, 1*time.Minute)
	defer SetRedis(nil)
	defer SetRPCCache(nil, 0)

	ri := &utils.RouterInstance{
		Config: &utils.RouterConfig{
			Name:        "test-router",
			Hostname:    addr,
			Username:    "test",
			SSHPoolSize: 0,
		},
		Router:      mockGrpcRouter{},
		HealthCheck: &utils.HealthCheck{Healthy: true},
	}

	riFailed := &utils.RouterInstance{
		Config: &utils.RouterConfig{
			Name:        "failed-router",
			Hostname:    "127.0.0.1:1",
			Username:    "test",
			SSHPoolSize: 0,
		},
		Router:      mockGrpcRouter{},
		HealthCheck: &utils.HealthCheck{Healthy: true},
	}

	riSSH := &utils.RouterInstance{
		Config: &utils.RouterConfig{
			Name:        "test-ssh-router",
			Hostname:    addr,
			Username:    "test",
			SSHPoolSize: 0,
		},
		Router:      mockGrpcRouter{},
		HealthCheck: &utils.HealthCheck{Healthy: true},
	}

	riExhausted := &utils.RouterInstance{
		Config: &utils.RouterConfig{
			Name:        "exhausted-router",
			Hostname:    addr,
			Username:    "test",
			SSHPoolSize: 0,
		},
		Router:      mockPoolExhaustedRouter{},
		HealthCheck: &utils.HealthCheck{Healthy: true},
	}

	rts := utils.RouterMap{ri, riFailed, riSSH, riExhausted}
	srv := NewLookingGlassService(context.Background(), rts)
	ctx := context.Background()

	infoResp, err := srv.GetInfo(ctx, connect.NewRequest(&emptypb.Empty{}))
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if infoResp.Msg.Version == "" {
		t.Errorf("empty version in GetInfo response")
	}

	rResp, err := srv.GetRouters(ctx, connect.NewRequest(&pb.GetRoutersRequest{Limit: 10, PageToken: 1}))
	if err != nil {
		t.Fatalf("GetRouters failed: %v", err)
	}
	if len(rResp.Msg.Routers) != 4 {
		t.Errorf("unexpected router catalog count: %d", len(rResp.Msg.Routers))
	}

	verifyCacheHit := func(t *testing.T, reqFunc func() (string, string, error)) {
		res1, cache1, err1 := reqFunc()
		if err1 != nil {
			t.Fatalf("initial request failed: %v", err1)
		}
		if cache1 == "HIT" {
			t.Errorf("expected cache MISS on first call")
		}

		res2, cache2, err2 := reqFunc()
		if err2 != nil {
			t.Fatalf("subsequent request failed: %v", err2)
		}
		if cache2 != "HIT" {
			t.Errorf("expected cache HIT on second call")
		}
		if res1 != res2 {
			t.Errorf("mismatched results on cache hit: %q vs %q", res1, res2)
		}
	}

	verifyCacheHit(t, func() (string, string, error) {
		resp, err := srv.Ping(ctx, connect.NewRequest(&pb.PingRequest{RouterId: 1, Target: "1.1.1.1"}))
		if err != nil {
			return "", "", err
		}
		return string(resp.Msg.Result), resp.Header().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		resp, err := srv.Traceroute(ctx, connect.NewRequest(&pb.TracerouteRequest{RouterId: 1, Target: "1.1.1.1"}))
		if err != nil {
			return "", "", err
		}
		return string(resp.Msg.Result), resp.Header().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		resp, err := srv.BGPSummary(ctx, connect.NewRequest(&pb.BGPSummaryRequest{RouterId: 1}))
		if err != nil {
			return "", "", err
		}
		return string(resp.Msg.Result), resp.Header().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		resp, err := srv.BGPRoute(ctx, connect.NewRequest(&pb.BGPRouteRequest{RouterId: 1, Target: "1.1.1.1"}))
		if err != nil {
			return "", "", err
		}
		return string(resp.Msg.Result), resp.Header().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		resp, err := srv.BGPCommunity(ctx, connect.NewRequest(&pb.BGPCommunityRequest{
			RouterId:  1,
			Community: &pb.BGPCommunity{Asn: 65000, Value: 100},
		}))
		if err != nil {
			return "", "", err
		}
		return string(resp.Msg.Result), resp.Header().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		resp, err := srv.BGPLargeCommunity(ctx, connect.NewRequest(&pb.BGPLargeCommunityRequest{
			RouterId: 1,
			Community: &pb.BGPLargeCommunity{
				GlobalAdmin: 65000,
				LocalData1:  100,
				LocalData2:  200,
			},
		}))
		if err != nil {
			return "", "", err
		}
		return string(resp.Msg.Result), resp.Header().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		resp, err := srv.BGPASPath(ctx, connect.NewRequest(&pb.BGPASPathRequest{RouterId: 1, Pattern: "203038"}))
		if err != nil {
			return "", "", err
		}
		return string(resp.Msg.Result), resp.Header().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		resp, err := srv.BGPPeerRoutes(ctx, connect.NewRequest(&pb.BGPPeerRoutesRequest{
			RouterId:  1,
			PeerIp:    "192.0.2.1",
			PeerName:  "peer_1",
			QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
		}))
		if err != nil {
			return "", "", err
		}
		return string(resp.Msg.Result), resp.Header().Get("X-Cache"), nil
	})

	_, err = srv.BGPSummary(ctx, connect.NewRequest(&pb.BGPSummaryRequest{RouterId: 3}))
	if err != nil {
		t.Errorf("BGPSummary SSH failed: %v", err)
	}
	_, err = srv.BGPRoute(ctx, connect.NewRequest(&pb.BGPRouteRequest{RouterId: 3, Target: "1.1.1.1"}))
	if err != nil {
		t.Errorf("BGPRoute SSH failed: %v", err)
	}
	_, err = srv.BGPCommunity(ctx, connect.NewRequest(&pb.BGPCommunityRequest{
		RouterId:  3,
		Community: &pb.BGPCommunity{Asn: 65000, Value: 100},
	}))
	if err != nil {
		t.Errorf("BGPCommunity SSH failed: %v", err)
	}
	_, err = srv.BGPLargeCommunity(ctx, connect.NewRequest(&pb.BGPLargeCommunityRequest{
		RouterId:  3,
		Community: &pb.BGPLargeCommunity{GlobalAdmin: 65000, LocalData1: 100, LocalData2: 100},
	}))
	if err != nil {
		t.Errorf("BGPLargeCommunity SSH failed: %v", err)
	}
	_, err = srv.BGPASPath(ctx, connect.NewRequest(&pb.BGPASPathRequest{RouterId: 3, Pattern: "203038"}))
	if err != nil {
		t.Errorf("BGPASPath SSH failed: %v", err)
	}
	_, err = srv.BGPPeerRoutes(ctx, connect.NewRequest(&pb.BGPPeerRoutesRequest{
		RouterId:  3,
		PeerIp:    "1.1.1.1",
		QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
	}))
	if err != nil {
		t.Errorf("BGPPeerRoutes SSH failed: %v", err)
	}

	_, err = srv.BGPCommunity(ctx, connect.NewRequest(&pb.BGPCommunityRequest{
		RouterId:  1,
		Community: &pb.BGPCommunity{Asn: 100000, Value: 100},
	}))
	if err == nil {
		t.Errorf("expected error for malformed community")
	}

	_, err = srv.BGPPeerRoutes(ctx, connect.NewRequest(&pb.BGPPeerRoutesRequest{
		RouterId: 1,
		PeerIp:   "invalid-peer-ip",
	}))
	if err == nil {
		t.Errorf("expected error for malformed peer IP")
	}

	_, err = srv.BGPASPath(ctx, connect.NewRequest(&pb.BGPASPathRequest{RouterId: 1, Pattern: "invalid-char-!"}))
	if err == nil {
		t.Errorf("expected error for malformed ASPath pattern")
	}

	_, err = srv.Ping(ctx, connect.NewRequest(&pb.PingRequest{RouterId: 99, Target: "1.1.1.1"}))
	if err == nil {
		t.Errorf("expected error for non-existent router ID")
	}

	_, err = srv.Ping(ctx, connect.NewRequest(&pb.PingRequest{RouterId: 1, Target: "invalid-target-ip"}))
	if err == nil {
		t.Errorf("expected error for invalid target on Ping")
	}

	_, err = srv.Traceroute(ctx, connect.NewRequest(&pb.TracerouteRequest{RouterId: 1, Target: "invalid-target-ip"}))
	if err == nil {
		t.Errorf("expected error for invalid target on Traceroute")
	}

	_, err = srv.BGPRoute(ctx, connect.NewRequest(&pb.BGPRouteRequest{RouterId: 1, Target: "invalid-target-ip"}))
	if err == nil {
		t.Errorf("expected error for invalid target on BGPRoute")
	}

	assertExecFailure := func(err error) {
		if err == nil {
			t.Errorf("expected execution error, got nil")
		}
		if !errors.Is(err, errs.ExecFailed) && !errors.Is(err, errs.ConnectionFailed) {
			t.Errorf("unexpected error kind: %v", err)
		}
	}

	_, err = srv.Ping(ctx, connect.NewRequest(&pb.PingRequest{RouterId: 2, Target: "1.1.1.1"}))
	assertExecFailure(err)

	_, err = srv.Traceroute(ctx, connect.NewRequest(&pb.TracerouteRequest{RouterId: 2, Target: "1.1.1.1"}))
	assertExecFailure(err)

	_, err = srv.BGPSummary(ctx, connect.NewRequest(&pb.BGPSummaryRequest{RouterId: 2}))
	assertExecFailure(err)

	_, err = srv.BGPRoute(ctx, connect.NewRequest(&pb.BGPRouteRequest{RouterId: 2, Target: "1.1.1.1"}))
	assertExecFailure(err)

	_, err = srv.BGPCommunity(ctx, connect.NewRequest(&pb.BGPCommunityRequest{
		RouterId:  2,
		Community: &pb.BGPCommunity{Asn: 65000, Value: 100},
	}))
	assertExecFailure(err)

	_, err = srv.BGPLargeCommunity(ctx, connect.NewRequest(&pb.BGPLargeCommunityRequest{
		RouterId:  2,
		Community: &pb.BGPLargeCommunity{GlobalAdmin: 65000, LocalData1: 100, LocalData2: 100},
	}))
	assertExecFailure(err)

	_, err = srv.BGPASPath(ctx, connect.NewRequest(&pb.BGPASPathRequest{RouterId: 2, Pattern: "203038"}))
	assertExecFailure(err)

	_, err = srv.BGPPeerRoutes(ctx, connect.NewRequest(&pb.BGPPeerRoutesRequest{
		RouterId:  2,
		PeerIp:    "1.1.1.1",
		QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
	}))
	assertExecFailure(err)

	assertPoolExhausted := func(err error) {
		if err == nil {
			t.Errorf("expected PoolExhausted error, got nil")
		}
		if !strings.Contains(err.Error(), "resource_exhausted") {
			t.Errorf("expected resource_exhausted, got: %v", err)
		}
	}

	_, err = srv.Ping(ctx, connect.NewRequest(&pb.PingRequest{RouterId: 4, Target: "1.1.1.1"}))
	assertPoolExhausted(err)

	_, err = srv.Traceroute(ctx, connect.NewRequest(&pb.TracerouteRequest{RouterId: 4, Target: "1.1.1.1"}))
	assertPoolExhausted(err)

	_, err = srv.BGPSummary(ctx, connect.NewRequest(&pb.BGPSummaryRequest{RouterId: 4}))
	assertPoolExhausted(err)

	_, err = srv.BGPRoute(ctx, connect.NewRequest(&pb.BGPRouteRequest{RouterId: 4, Target: "1.1.1.1"}))
	assertPoolExhausted(err)

	_, err = srv.BGPCommunity(ctx, connect.NewRequest(&pb.BGPCommunityRequest{
		RouterId:  4,
		Community: &pb.BGPCommunity{Asn: 65000, Value: 100},
	}))
	assertPoolExhausted(err)

	_, err = srv.BGPLargeCommunity(ctx, connect.NewRequest(&pb.BGPLargeCommunityRequest{
		RouterId:  4,
		Community: &pb.BGPLargeCommunity{GlobalAdmin: 65000, LocalData1: 100, LocalData2: 100},
	}))
	assertPoolExhausted(err)

	_, err = srv.BGPASPath(ctx, connect.NewRequest(&pb.BGPASPathRequest{RouterId: 4, Pattern: "203038"}))
	assertPoolExhausted(err)

	_, err = srv.BGPPeerRoutes(ctx, connect.NewRequest(&pb.BGPPeerRoutesRequest{
		RouterId:  4,
		PeerIp:    "1.1.1.1",
		QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
	}))
	assertPoolExhausted(err)

	// Test that querying for REJECTED routes on a BMP-active router returns successfully with empty routes and never falls back to SSH
	respRejected, err := srv.BGPPeerRoutes(ctx, connect.NewRequest(&pb.BGPPeerRoutesRequest{
		RouterId:  1,
		PeerIp:    "192.0.2.1",
		PeerName:  "peer_1",
		QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_REJECTED,
	}))
	if err != nil {
		t.Errorf("expected success for REJECTED routes under BMP, got error: %v", err)
	} else if len(respRejected.Msg.Parsed.Paths) != 0 {
		t.Errorf("expected 0 rejected routes under BMP, got %d", len(respRejected.Msg.Parsed.Paths))
	}

	_, err = srv.GetRouters(ctx, connect.NewRequest(&pb.GetRoutersRequest{Limit: 0, PageToken: 0}))
	if err != nil {
		t.Errorf("expected success on invalid limit/page GetRouters, got %v", err)
	}
	_, err = srv.GetRouters(ctx, connect.NewRequest(&pb.GetRoutersRequest{Limit: 10, PageToken: 99}))
	if err != nil {
		t.Errorf("expected success on page token out of bounds GetRouters, got %v", err)
	}
}
