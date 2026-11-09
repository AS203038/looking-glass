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

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectinprocess"
	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/routers"
	"github.com/AS203038/looking-glass/pkg/utils"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
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
	server := newServer(srv)
	cli := lookingglassconnect.NewLookingGlassServiceClient(
		connect.NewClient(connectinprocess.New(server)),
	)
	ctx := context.Background()

	infoResp, err := cli.GetInfo(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if infoResp.Version == "" {
		t.Errorf("empty version in GetInfo response")
	}

	rResp, err := cli.GetRouters(ctx, &pb.GetRoutersRequest{Limit: 10, PageToken: 1})
	if err != nil {
		t.Fatalf("GetRouters failed: %v", err)
	}
	if len(rResp.Routers) != 4 {
		t.Errorf("unexpected router catalog count: %d", len(rResp.Routers))
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
		callCtx, info := connect.NewClientContext(ctx)
		resp, err := cli.Ping(callCtx, &pb.PingRequest{RouterId: 1, Target: "1.1.1.1"})
		if err != nil {
			return "", "", err
		}
		return string(resp.Result), info.ResponseHeader().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		callCtx, info := connect.NewClientContext(ctx)
		resp, err := cli.Traceroute(callCtx, &pb.TracerouteRequest{RouterId: 1, Target: "1.1.1.1"})
		if err != nil {
			return "", "", err
		}
		return string(resp.Result), info.ResponseHeader().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		callCtx, info := connect.NewClientContext(ctx)
		resp, err := cli.BGPSummary(callCtx, &pb.BGPSummaryRequest{RouterId: 1})
		if err != nil {
			return "", "", err
		}
		return string(resp.Result), info.ResponseHeader().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		callCtx, info := connect.NewClientContext(ctx)
		resp, err := cli.BGPRoute(callCtx, &pb.BGPRouteRequest{RouterId: 1, Target: "1.1.1.1"})
		if err != nil {
			return "", "", err
		}
		return string(resp.Result), info.ResponseHeader().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		callCtx, info := connect.NewClientContext(ctx)
		resp, err := cli.BGPCommunity(callCtx, &pb.BGPCommunityRequest{
			RouterId:  1,
			Community: &pb.BGPCommunity{Asn: 65000, Value: 100},
		})
		if err != nil {
			return "", "", err
		}
		return string(resp.Result), info.ResponseHeader().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		callCtx, info := connect.NewClientContext(ctx)
		resp, err := cli.BGPLargeCommunity(callCtx, &pb.BGPLargeCommunityRequest{
			RouterId: 1,
			Community: &pb.BGPLargeCommunity{
				GlobalAdmin: 65000,
				LocalData1:  100,
				LocalData2:  200,
			},
		})
		if err != nil {
			return "", "", err
		}
		return string(resp.Result), info.ResponseHeader().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		callCtx, info := connect.NewClientContext(ctx)
		resp, err := cli.BGPASPath(callCtx, &pb.BGPASPathRequest{RouterId: 1, Pattern: "203038"})
		if err != nil {
			return "", "", err
		}
		return string(resp.Result), info.ResponseHeader().Get("X-Cache"), nil
	})

	verifyCacheHit(t, func() (string, string, error) {
		callCtx, info := connect.NewClientContext(ctx)
		resp, err := cli.BGPPeerRoutes(callCtx, &pb.BGPPeerRoutesRequest{
			RouterId:  1,
			PeerIp:    "192.0.2.1",
			PeerName:  "peer_1",
			QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
		})
		if err != nil {
			return "", "", err
		}
		return string(resp.Result), info.ResponseHeader().Get("X-Cache"), nil
	})

	_, err = cli.BGPSummary(ctx, &pb.BGPSummaryRequest{RouterId: 3})
	if err != nil {
		t.Errorf("BGPSummary SSH failed: %v", err)
	}
	_, err = cli.BGPRoute(ctx, &pb.BGPRouteRequest{RouterId: 3, Target: "1.1.1.1"})
	if err != nil {
		t.Errorf("BGPRoute SSH failed: %v", err)
	}
	_, err = cli.BGPCommunity(ctx, &pb.BGPCommunityRequest{
		RouterId:  3,
		Community: &pb.BGPCommunity{Asn: 65000, Value: 100},
	})
	if err != nil {
		t.Errorf("BGPCommunity SSH failed: %v", err)
	}
	_, err = cli.BGPLargeCommunity(ctx, &pb.BGPLargeCommunityRequest{
		RouterId:  3,
		Community: &pb.BGPLargeCommunity{GlobalAdmin: 65000, LocalData1: 100, LocalData2: 100},
	})
	if err != nil {
		t.Errorf("BGPLargeCommunity SSH failed: %v", err)
	}
	_, err = cli.BGPASPath(ctx, &pb.BGPASPathRequest{RouterId: 3, Pattern: "203038"})
	if err != nil {
		t.Errorf("BGPASPath SSH failed: %v", err)
	}
	_, err = cli.BGPPeerRoutes(ctx, &pb.BGPPeerRoutesRequest{
		RouterId:  3,
		PeerIp:    "1.1.1.1",
		QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
	})
	if err != nil {
		t.Errorf("BGPPeerRoutes SSH failed: %v", err)
	}

	_, err = cli.BGPCommunity(ctx, &pb.BGPCommunityRequest{
		RouterId:  1,
		Community: &pb.BGPCommunity{Asn: 100000, Value: 100},
	})
	if err == nil {
		t.Errorf("expected error for malformed community")
	}

	_, err = cli.BGPPeerRoutes(ctx, &pb.BGPPeerRoutesRequest{
		RouterId: 1,
		PeerIp:   "invalid-peer-ip",
	})
	if err == nil {
		t.Errorf("expected error for malformed peer IP")
	}

	_, err = cli.BGPASPath(ctx, &pb.BGPASPathRequest{RouterId: 1, Pattern: "invalid-char-!"})
	if err == nil {
		t.Errorf("expected error for malformed ASPath pattern")
	}

	_, err = cli.Ping(ctx, &pb.PingRequest{RouterId: 99, Target: "1.1.1.1"})
	if err == nil {
		t.Errorf("expected error for non-existent router ID")
	}

	_, err = cli.Ping(ctx, &pb.PingRequest{RouterId: 1, Target: "invalid-target-ip"})
	if err == nil {
		t.Errorf("expected error for invalid target on Ping")
	}

	_, err = cli.Traceroute(ctx, &pb.TracerouteRequest{RouterId: 1, Target: "invalid-target-ip"})
	if err == nil {
		t.Errorf("expected error for invalid target on Traceroute")
	}

	_, err = cli.BGPRoute(ctx, &pb.BGPRouteRequest{RouterId: 1, Target: "invalid-target-ip"})
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

	_, err = cli.Ping(ctx, &pb.PingRequest{RouterId: 2, Target: "1.1.1.1"})
	assertExecFailure(err)

	_, err = cli.Traceroute(ctx, &pb.TracerouteRequest{RouterId: 2, Target: "1.1.1.1"})
	assertExecFailure(err)

	_, err = cli.BGPSummary(ctx, &pb.BGPSummaryRequest{RouterId: 2})
	assertExecFailure(err)

	_, err = cli.BGPRoute(ctx, &pb.BGPRouteRequest{RouterId: 2, Target: "1.1.1.1"})
	assertExecFailure(err)

	_, err = cli.BGPCommunity(ctx, &pb.BGPCommunityRequest{
		RouterId:  2,
		Community: &pb.BGPCommunity{Asn: 65000, Value: 100},
	})
	assertExecFailure(err)

	_, err = cli.BGPLargeCommunity(ctx, &pb.BGPLargeCommunityRequest{
		RouterId:  2,
		Community: &pb.BGPLargeCommunity{GlobalAdmin: 65000, LocalData1: 100, LocalData2: 100},
	})
	assertExecFailure(err)

	_, err = cli.BGPASPath(ctx, &pb.BGPASPathRequest{RouterId: 2, Pattern: "203038"})
	assertExecFailure(err)

	_, err = cli.BGPPeerRoutes(ctx, &pb.BGPPeerRoutesRequest{
		RouterId:  2,
		PeerIp:    "1.1.1.1",
		QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
	})
	assertExecFailure(err)

	assertPoolExhausted := func(err error) {
		if err == nil {
			t.Errorf("expected PoolExhausted error, got nil")
		}
		if !strings.Contains(err.Error(), "resource_exhausted") {
			t.Errorf("expected resource_exhausted, got: %v", err)
		}
	}

	_, err = cli.Ping(ctx, &pb.PingRequest{RouterId: 4, Target: "1.1.1.1"})
	assertPoolExhausted(err)

	_, err = cli.Traceroute(ctx, &pb.TracerouteRequest{RouterId: 4, Target: "1.1.1.1"})
	assertPoolExhausted(err)

	_, err = cli.BGPSummary(ctx, &pb.BGPSummaryRequest{RouterId: 4})
	assertPoolExhausted(err)

	_, err = cli.BGPRoute(ctx, &pb.BGPRouteRequest{RouterId: 4, Target: "1.1.1.1"})
	assertPoolExhausted(err)

	_, err = cli.BGPCommunity(ctx, &pb.BGPCommunityRequest{
		RouterId:  4,
		Community: &pb.BGPCommunity{Asn: 65000, Value: 100},
	})
	assertPoolExhausted(err)

	_, err = cli.BGPLargeCommunity(ctx, &pb.BGPLargeCommunityRequest{
		RouterId:  4,
		Community: &pb.BGPLargeCommunity{GlobalAdmin: 65000, LocalData1: 100, LocalData2: 100},
	})
	assertPoolExhausted(err)

	_, err = cli.BGPASPath(ctx, &pb.BGPASPathRequest{RouterId: 4, Pattern: "203038"})
	assertPoolExhausted(err)

	_, err = cli.BGPPeerRoutes(ctx, &pb.BGPPeerRoutesRequest{
		RouterId:  4,
		PeerIp:    "1.1.1.1",
		QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
	})
	assertPoolExhausted(err)

	// Test that querying for REJECTED routes on a BMP-active router returns successfully with empty routes and never falls back to SSH
	respRejected, err := cli.BGPPeerRoutes(ctx, &pb.BGPPeerRoutesRequest{
		RouterId:  1,
		PeerIp:    "192.0.2.1",
		PeerName:  "peer_1",
		QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_REJECTED,
	})
	if err != nil {
		t.Errorf("expected success for REJECTED routes under BMP, got error: %v", err)
	} else if len(respRejected.Parsed.Paths) != 0 {
		t.Errorf("expected 0 rejected routes under BMP, got %d", len(respRejected.Parsed.Paths))
	}

	_, err = cli.GetRouters(ctx, &pb.GetRoutersRequest{Limit: 0, PageToken: 0})
	if err != nil {
		t.Errorf("expected success on invalid limit/page GetRouters, got %v", err)
	}
	_, err = cli.GetRouters(ctx, &pb.GetRoutersRequest{Limit: 10, PageToken: 99})
	if err != nil {
		t.Errorf("expected success on page token out of bounds GetRouters, got %v", err)
	}
}

// startExecRecorder starts an SSH server that answers each exec request and sends its command to cmds.
func startExecRecorder(t *testing.T, cmds chan<- string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	go func() {
		for {
			conn, err := ln.Accept()
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
					ch, chReqs, err := newChan.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer ch.Close()
						for req := range chReqs {
							req.Reply(true, nil)
							if req.Type == "exec" {
								var payload struct{ Command string }
								_ = ssh.Unmarshal(req.Payload, &payload)
								cmds <- payload.Command
								ch.Write([]byte("output\n"))
								ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								return
							}
						}
					}()
				}
			}()
		}
	}()
	return ln.Addr().String()
}

// startRESPRecorder starts a Redis stand-in that sends each request to reqs. BMP is active for routers named bmp-*, with no routes.
func startRESPRecorder(t *testing.T, reqs chan<- string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					req := strings.ToLower(string(buf[:n]))
					reqs <- req
					switch {
					case strings.Contains(req, "hello"):
						c.Write([]byte("-ERR unknown command\r\n"))
					case strings.Contains(req, "setinfo"):
						c.Write([]byte("+OK\r\n+OK\r\n"))
					case strings.Contains(req, "hlen") && strings.Contains(req, "bmp-"):
						c.Write([]byte(":1\r\n"))
					case strings.Contains(req, "hlen"):
						c.Write([]byte(":0\r\n"))
					case strings.Contains(req, "hgetall"):
						c.Write([]byte("*0\r\n"))
					case strings.Contains(req, "get"):
						c.Write([]byte("$-1\r\n"))
					default:
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

// A link-local peer from a Junos BGP summary keeps its interface up to the router command, also on a BMP router.
func TestBGPPeerRoutesLinkLocalPeer(t *testing.T) {
	cmds := make(chan string, 8)
	reqs := make(chan string, 256)
	client := redis.NewClient(&redis.Options{Addr: startRESPRecorder(t, reqs), Protocol: 2})
	defer client.Close()
	SetRedis(client)
	SetRPCCache(client, time.Minute)
	defer SetRedis(nil)
	defer SetRPCCache(nil, 0)

	addr := startExecRecorder(t, cmds)
	router := func(name string) *utils.RouterInstance {
		return &utils.RouterInstance{
			Config:      &utils.RouterConfig{Name: name, Hostname: addr, Username: "test"},
			Router:      routers.Get("juniper_junos"),
			HealthCheck: &utils.HealthCheck{Healthy: true},
		}
	}
	server := connect.NewServer()
	lookingglassconnect.RegisterLookingGlassServiceHandler(server,
		NewLookingGlassService(context.Background(), utils.RouterMap{router("ssh-router"), router("bmp-router")}))
	cli := lookingglassconnect.NewLookingGlassServiceClient(connect.NewClient(connectinprocess.New(server)))
	ask := func(routerID int64, peer string) error {
		_, err := cli.BGPPeerRoutes(context.Background(), &pb.BGPPeerRoutesRequest{
			RouterId: routerID, PeerIp: peer, QueryType: pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED,
		})
		return err
	}
	const scoped = "fe80::5a86:70ff:fe78:e0d5%et-0/0/1:0.0"

	for _, peer := range []string{scoped, "192.0.2.1"} {
		if err := ask(1, peer); err != nil {
			t.Fatalf("BGPPeerRoutes(%q): %v", peer, err)
		}
		select {
		case got := <-cmds:
			if want := "show route receive-protocol bgp " + peer; got != want {
				t.Errorf("command %q, want %q", got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("no router command for %q", peer)
		}
	}

	// The interface goes unquoted into the router command, and only a link-local address has one.
	for _, peer := range []string{
		"fe80::1%et-0/0/1;show configuration",
		"fe80::1%et 0",
		`fe80::1%"x"`,
		"fe80::1%",
		"2001:db8::1%et-0/0/1:0.0",
		"192.0.2.1%eth0",
	} {
		if err := ask(1, peer); !errors.Is(err, errs.IPInvalid) {
			t.Errorf("BGPPeerRoutes(%q) = %v, want %v", peer, err, errs.IPInvalid)
		}
	}
	select {
	case got := <-cmds:
		t.Errorf("router got command %q for a rejected peer", got)
	default:
	}

	// BMP has no interface in its key, so it cannot select the peer: a scoped peer goes to the router.
	for len(reqs) > 0 {
		<-reqs
	}
	if err := ask(2, scoped); err != nil {
		t.Fatalf("BGPPeerRoutes(%q) on a BMP router: %v", scoped, err)
	}
	select {
	case got := <-cmds:
		if want := "show route receive-protocol bgp " + scoped; got != want {
			t.Errorf("command %q, want %q", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Errorf("no router command for %q on a BMP router", scoped)
	}
	for len(reqs) > 0 {
		if req := <-reqs; strings.Contains(req, "hgetall") {
			t.Errorf("BMP lookup for a scoped peer: %q", req)
		}
	}

	// A peer without an interface stays on BMP.
	const unscoped = "fe80::5a86:70ff:fe78:e0d5"
	if err := ask(2, unscoped); err != nil {
		t.Fatalf("BGPPeerRoutes(%q) on a BMP router: %v", unscoped, err)
	}
	select {
	case got := <-cmds:
		t.Errorf("router got command %q for a peer on BMP", got)
	default:
	}
	ribKey := "\r\nlg:bmp:rib:bmp-router:" + unscoped + "\r\n"
	for found := false; !found; {
		select {
		case req := <-reqs:
			found = strings.Contains(req, "hgetall") && strings.Contains(req, ribKey)
		default:
			t.Fatalf("BMP lookup did not use the key %q", strings.TrimSpace(ribKey))
		}
	}
}
