package bmp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/redis/go-redis/v9"
)

// TestBMPRedisOperations verifies standard BMP Redis peer, route, and index storage and lookup operations.
func TestBMPRedisOperations(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock redis: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					req := string(buf[:n])
					commands := strings.Split(req, "*")
					for _, cmd := range commands {
						if cmd == "" {
							continue
						}
						lowerCmd := strings.ToLower(cmd)
						if strings.Contains(lowerCmd, "hello") {
							c.Write([]byte("%7\r\n$6\r\nserver\r\n$5\r\nredis\r\n$7\r\nversion\r\n$5\r\n7.0.0\r\n$5\r\nproto\r\n:2\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"))
						} else if strings.Contains(lowerCmd, "setinfo") {
							c.Write([]byte("+OK\r\n"))
						} else if strings.Contains(lowerCmd, "hgetall") {
							if strings.Contains(lowerCmd, "peers") {
								val := `{"peer_ip":"192.0.2.1","peer_asn":65001,"state":"established","uptime_seconds":1,"timestamp":123456}`
								c.Write([]byte(fmt.Sprintf("*2\r\n$9\r\n192.0.2.1\r\n$%d\r\n%s\r\n", len(val), val)))
							} else if strings.Contains(lowerCmd, "rib") {
								val := `{"prefix":"192.0.2.0/24","as_path":[65001],"next_hop":"192.0.2.1","communities":["65001:100"],"large_communities":["65001:100:10"]}`
								c.Write([]byte(fmt.Sprintf("*2\r\n$12\r\n192.0.2.0/24\r\n$%d\r\n%s\r\n", len(val), val)))
							} else {
								c.Write([]byte("*0\r\n"))
							}
						} else if strings.Contains(lowerCmd, "hget") {
							if strings.Contains(lowerCmd, "peers") {
								val := `{"peer_ip":"192.0.2.1","peer_asn":65001,"state":"established","uptime_seconds":1,"timestamp":123456}`
								c.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)))
							} else {
								c.Write([]byte("$-1\r\n"))
							}
						} else if strings.Contains(lowerCmd, "hset") || strings.Contains(lowerCmd, "hdel") || strings.Contains(lowerCmd, "hlen") || strings.Contains(lowerCmd, "del") {
							c.Write([]byte(":1\r\n"))
						} else if strings.Contains(lowerCmd, "hscan") {
							c.Write([]byte("*2\r\n$1\r\n0\r\n*0\r\n"))
						} else if strings.Contains(lowerCmd, "client") {
							c.Write([]byte("+OK\r\n"))
						} else if strings.Contains(lowerCmd, "ping") {
							c.Write([]byte("+PONG\r\n"))
						}
					}
				}
			}(conn)
		}
	}()

	client := redis.NewClient(&redis.Options{
		Addr: ln.Addr().String(),
	})
	defer client.Close()

	ctx := context.Background()

	err = UpdateBMPPeerStatus(ctx, client, "test-router", "192.0.2.1", 65001, "established", "1.1.1.1")
	if err != nil {
		t.Fatalf("failed to update peer status: %v", err)
	}

	peers, err := GetBMPPeers(ctx, client, "test-router")
	if err != nil {
		t.Fatalf("failed to get peers: %v", err)
	}
	if len(peers) == 0 {
		t.Errorf("expected peers to be returned")
	}

	up := &BGPUpdate{
		AnnouncedPrefixes: []string{"192.0.2.0/24"},
		WithdrawnPrefixes: []string{"198.51.100.0/24"},
		ASPath:            []uint32{65001},
		NextHop:           "192.0.2.1",
	}
	err = UpdateBMPRib(ctx, client, "test-router", "192.0.2.1", up)
	if err != nil {
		t.Fatalf("failed to update RIB: %v", err)
	}

	routes, err := GetBMPPeerRoutes(ctx, client, "test-router", "192.0.2.1", "received")
	if err != nil {
		t.Fatalf("failed to get peer routes: %v", err)
	}
	_ = routes

	_, err = GetBMPPeerRoutes(ctx, client, "test-router", "192.0.2.1", "rejected")
	if err != nil {
		t.Fatalf("failed to get rejected routes: %v", err)
	}
	_, err = GetBMPPeerRoutes(ctx, client, "test-router", "192.0.2.1", "advertised")
	if err != nil {
		t.Fatalf("failed to get advertised routes: %v", err)
	}

	_, err = FindBMPRoutesByPrefix(ctx, client, "test-router", "192.0.2.0/24")
	if err != nil {
		t.Fatalf("failed to find routes by prefix: %v", err)
	}

	scanRoutes, err := FindBMPRoutesByPrefix(ctx, client, "test-router", "192.0.2.1")
	if err != nil {
		t.Fatalf("failed prefix scan lookup: %v", err)
	}
	_ = scanRoutes

	active := IsBMPActive(ctx, client, "test-router")
	if !active {
		t.Errorf("expected BMP to report as active")
	}
}

// TestNilRedisBMP verifies all BMP Redis operations handle nil Redis client gracefully.
func TestNilRedisBMP(t *testing.T) {
	ctx := context.Background()
	_ = UpdateBMPPeerStatus(ctx, nil, "test", "1.1.1.1", 65000, "established", "1.1.1.1")
	_ = UpdateBMPRib(ctx, nil, "test", "1.1.1.1", &BGPUpdate{})
	_, _ = GetBMPPeers(ctx, nil, "test")
	_, _ = GetBMPPeerRoutes(ctx, nil, "test", "1.1.1.1", "received")
	_, _, _ = GetBMPPeerRoutesPaginated(ctx, nil, "test", "1.1.1.1", "received", 0, 50)
	_, _ = FindBMPRoutesByPrefix(ctx, nil, "test", "1.1.1.1")
	_ = IsBMPActive(ctx, nil, "test")
	_, _ = FindBMPRoutesByCommunity(ctx, nil, "test", "65000:100")
	_, _ = FindBMPRoutesByLargeCommunity(ctx, nil, "test", "65000:100:10")
	_, _ = FindBMPRoutesByASPath(ctx, nil, "test", "_65000_")
}

// TestBMPExtractASNs verifies extractASNsFromPattern correctly extracts and deduplicates ASNs from AS path regex.
func TestBMPExtractASNs(t *testing.T) {
	tests := []struct {
		pattern  string
		expected []uint32
	}{
		{"_65000_", []uint32{65000}},
		{"^65000 64512$", []uint32{65000, 64512}},
		{"_203038_65000_203038_", []uint32{203038, 65000}},
		{"invalid", nil},
	}
	for _, tc := range tests {
		got := extractASNsFromPattern(tc.pattern)
		if len(got) != len(tc.expected) {
			t.Errorf("extractASNsFromPattern(%q) returned %v, expected %v", tc.pattern, got, tc.expected)
			continue
		}
		for i := range got {
			if got[i] != tc.expected[i] {
				t.Errorf("extractASNsFromPattern(%q) returned %v, expected %v", tc.pattern, got, tc.expected)
				break
			}
		}
	}
}

// TestBMPMatchASPathRegex verifies matchASPathRegex correctly filters routes based on regex match on AS paths.
func TestBMPMatchASPathRegex(t *testing.T) {
	routes := []RouteInfo{
		{Prefix: "1.1.1.0/24", ASPath: []uint32{65000, 64512, 203038}},
		{Prefix: "2.2.2.0/24", ASPath: []uint32{65000}},
		{Prefix: "3.3.3.0/24", ASPath: []uint32{}},
	}

	tests := []struct {
		pattern string
		matches []string
	}{
		{"_65000_", []string{"1.1.1.0/24", "2.2.2.0/24"}},
		{"_203038$", []string{"1.1.1.0/24"}},
		{"^65000 64512", []string{"1.1.1.0/24"}},
		{"_64512_", []string{"1.1.1.0/24"}},
		{"_999_", nil},
	}

	for _, tc := range tests {
		matched, err := matchASPathRegex(tc.pattern, routes)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.pattern, err)
		}
		var got []string
		for _, r := range matched {
			got = append(got, r.Prefix)
		}
		if len(got) != len(tc.matches) {
			t.Errorf("matchASPathRegex(%q) matched %v, expected %v", tc.pattern, got, tc.matches)
			continue
		}
		for i := range got {
			if got[i] != tc.matches[i] {
				t.Errorf("matchASPathRegex(%q) matched %v, expected %v", tc.pattern, got, tc.matches)
				break
			}
		}
	}
}

// TestGenerateIPPrefixes verifies generateIPPrefixes correctly generates enclosing CIDR candidates for IPv4 and IPv6.
func TestGenerateIPPrefixes(t *testing.T) {
	v4Candidates := generateIPPrefixes("192.0.2.1")
	if len(v4Candidates) != 33 {
		t.Errorf("expected 33 IPv4 enclosing prefix candidates, got %d", len(v4Candidates))
	}
	if v4Candidates[0] != "192.0.2.1/32" || v4Candidates[32] != "0.0.0.0/0" {
		t.Errorf("unexpected IPv4 candidates bounds: [0]=%s, [32]=%s", v4Candidates[0], v4Candidates[32])
	}

	v6Candidates := generateIPPrefixes("2001:db8::1")
	if len(v6Candidates) != 129 {
		t.Errorf("expected 129 IPv6 enclosing prefix candidates, got %d", len(v6Candidates))
	}
	if v6Candidates[0] != "2001:db8::1/128" || v6Candidates[128] != "::/0" {
		t.Errorf("unexpected IPv6 candidates bounds: [0]=%s, [128]=%s", v6Candidates[0], v6Candidates[128])
	}

	invalid := generateIPPrefixes("invalid-ip")
	if invalid != nil {
		t.Errorf("expected nil candidates for invalid IP, got %v", invalid)
	}
}

// TestFindRouterByRemoteIP verifies findRouterByRemoteIP matches remote address hostname or resolved DNS address with configuration.
func TestFindRouterByRemoteIP(t *testing.T) {
	rts := utils.RouterMap{
		&utils.RouterInstance{
			Config: &utils.RouterConfig{
				Name:     "localhost-test",
				Hostname: "127.0.0.1",
			},
		},
		&utils.RouterInstance{
			Config: &utils.RouterConfig{
				Name:     "dns-test",
				Hostname: "localhost",
			},
		},
		&utils.RouterInstance{
			Config: &utils.RouterConfig{
				Name:     "mismatch-test",
				Hostname: "198.51.100.1",
			},
		},
	}
	matched := findRouterByRemoteIP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 11019}, rts)
	if matched != "localhost-test" {
		t.Errorf("expected IP match to find 'localhost-test', got %q", matched)
	}

	rtsDns := utils.RouterMap{rts[1], rts[2]}
	matchedDns := findRouterByRemoteIP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 11019}, rtsDns)
	if matchedDns != "dns-test" {
		t.Errorf("expected DNS match to find 'dns-test', got %q", matchedDns)
	}

	rtsMismatch := utils.RouterMap{rts[2]}
	matchedMismatch := findRouterByRemoteIP(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 11019}, rtsMismatch)
	if matchedMismatch != "" {
		t.Errorf("expected no match, got %q", matchedMismatch)
	}
}

// TestBMPReconciliation verifies BGP PeerUp triggers self-healing flush on existing RIB routes and index cleanup.
func TestBMPReconciliation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock redis: %v", err)
	}
	defer ln.Close()

	var receivedDel, receivedHscan bool

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					req := string(buf[:n])
					commands := strings.Split(req, "*")
					for _, cmd := range commands {
						if cmd == "" {
							continue
						}
						lowerCmd := strings.ToLower(cmd)
						if strings.Contains(lowerCmd, "hello") {
							c.Write([]byte("%7\r\n$6\r\nserver\r\n$5\r\nredis\r\n$7\r\nversion\r\n$5\r\n7.0.0\r\n$5\r\nproto\r\n:2\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"))
						} else if strings.Contains(lowerCmd, "setinfo") {
							c.Write([]byte("+OK\r\n"))
						} else if strings.Contains(lowerCmd, "hscan") {
							receivedHscan = true
							c.Write([]byte("*2\r\n$1\r\n0\r\n*0\r\n"))
						} else if strings.Contains(lowerCmd, "hget") {
							c.Write([]byte("$-1\r\n"))
						} else if strings.Contains(lowerCmd, "hset") || strings.Contains(lowerCmd, "hdel") || strings.Contains(lowerCmd, "hlen") || strings.Contains(lowerCmd, "del") {
							if strings.Contains(lowerCmd, "del") {
								receivedDel = true
							}
							c.Write([]byte(":1\r\n"))
						} else if strings.Contains(lowerCmd, "client") {
							c.Write([]byte("+OK\r\n"))
						} else {
							c.Write([]byte("+OK\r\n"))
						}
					}
				}
			}(conn)
		}
	}()

	client := redis.NewClient(&redis.Options{
		Addr: ln.Addr().String(),
	})
	defer client.Close()

	ctx := context.Background()
	err = UpdateBMPPeerStatus(ctx, client, "test-router", "192.0.2.1", 65001, "established", "1.1.1.1")
	if err != nil {
		t.Fatalf("failed to update peer status: %v", err)
	}

	if !receivedHscan {
		t.Errorf("expected HSCAN to be called for index cleanup")
	}
	if !receivedDel {
		t.Errorf("expected DEL to be called for peer RIB cleanup")
	}
}

// TestBMPRedisPaginationASPathAndBatching verifies multi-page HScan filtering, AS-path HScan fallback, and batched HMGet fetching.
func TestBMPRedisPaginationASPathAndBatching(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock redis: %v", err)
	}
	defer ln.Close()

	var hscanCalls, hmgetCalls atomic.Int32
	routeAccepted := `{"prefix":"192.0.2.0/24","as_path":[65001,203038],"next_hop":"192.0.2.1","accepted":true,"rejected":false}`
	routeRejected := `{"prefix":"198.51.100.0/24","as_path":[65002],"next_hop":"192.0.2.1","accepted":false,"rejected":true}`

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if !strings.HasPrefix(line, "*") {
						continue
					}
					n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
					args := make([]string, 0, n)
					for i := 0; i < n; i++ {
						szLine, err := br.ReadString('\n')
						if err != nil {
							return
						}
						sz, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(szLine, "$")))
						argBuf := make([]byte, sz+2)
						if _, err := io.ReadFull(br, argBuf); err != nil {
							return
						}
						args = append(args, string(argBuf[:sz]))
					}
					if len(args) == 0 {
						continue
					}
					switch strings.ToLower(args[0]) {
					case "hello":
						c.Write([]byte("%7\r\n$6\r\nserver\r\n$5\r\nredis\r\n$7\r\nversion\r\n$5\r\n7.0.0\r\n$5\r\nproto\r\n:2\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"))
					case "hgetall":
						peerJSON := `{"peer_ip":"192.0.2.1","peer_asn":65001,"state":"established"}`
						c.Write([]byte(fmt.Sprintf("*2\r\n$9\r\n192.0.2.1\r\n$%d\r\n%s\r\n", len(peerJSON), peerJSON)))
					case "smembers", "sinter":
						m := "192.0.2.1:192.0.2.0/24"
						c.Write([]byte(fmt.Sprintf("*1\r\n$%d\r\n%s\r\n", len(m), m)))
					case "hlen":
						c.Write([]byte(":1\r\n"))
					case "hget":
						c.Write([]byte("$-1\r\n"))
					case "hscan":
						hscanCalls.Add(1)
						cursor := "0"
						if len(args) > 2 {
							cursor = args[2]
						}
						if cursor == "0" {
							c.Write([]byte(fmt.Sprintf("*2\r\n$2\r\n42\r\n*2\r\n$12\r\n192.0.2.0/24\r\n$%d\r\n%s\r\n", len(routeAccepted), routeAccepted)))
						} else {
							c.Write([]byte(fmt.Sprintf("*2\r\n$1\r\n0\r\n*2\r\n$15\r\n198.51.100.0/24\r\n$%d\r\n%s\r\n", len(routeRejected), routeRejected)))
						}
					case "hmget":
						hmgetCalls.Add(1)
						fields := len(args) - 2
						if fields < 1 {
							fields = 1
						}
						var sb strings.Builder
						sb.WriteString(fmt.Sprintf("*%d\r\n", fields))
						sb.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(routeAccepted), routeAccepted))
						for i := 1; i < fields; i++ {
							sb.WriteString("$-1\r\n")
						}
						c.Write([]byte(sb.String()))
					default:
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	client := redis.NewClient(&redis.Options{Addr: ln.Addr().String()})
	defer client.Close()
	ctx := context.Background()

	// Filtered paginated lookup advances across an empty-match round.
	routes, nextCursor, err := GetBMPPeerRoutesPaginated(ctx, client, "rt1", "192.0.2.1", "rejected", 0, 10)
	if err != nil {
		t.Fatalf("GetBMPPeerRoutesPaginated failed: %v", err)
	}
	if len(routes) != 1 || routes[0].Prefix != "198.51.100.0/24" || nextCursor != 0 {
		t.Fatalf("unexpected paginated rejected routes: %+v, cursor=%d", routes, nextCursor)
	}
	if hscanCalls.Load() < 2 {
		t.Errorf("expected at least 2 HScan calls when skipping empty-match page, got %d", hscanCalls.Load())
	}

	// Pattern with no numeric ASNs streams via HScan.
	asRoutes, err := FindBMPRoutesByASPath(ctx, client, "rt1", ".*")
	if err != nil {
		t.Fatalf("FindBMPRoutesByASPath fallback failed: %v", err)
	}
	if len(asRoutes) != 1 || asRoutes[0].Prefix != "192.0.2.0/24" {
		t.Fatalf("unexpected ASPath fallback routes: %+v", asRoutes)
	}

	// >500 members are fetched in multiple HMGet batches.
	members := make([]string, 550)
	for i := range members {
		members[i] = fmt.Sprintf("192.0.2.1:10.0.%d.0/24", i)
	}
	batched, err := fetchRoutesFromIndexMembers(ctx, client, "rt1", members)
	if err != nil {
		t.Fatalf("fetchRoutesFromIndexMembers failed: %v", err)
	}
	if hmgetCalls.Load() != 2 || len(batched) != 2 {
		t.Errorf("expected 2 HMGet batches and 2 decoded routes, got hmgetCalls=%d len=%d", hmgetCalls.Load(), len(batched))
	}

	// Verify community, large-community, ASN-indexed ASPath lookups, and route overwrite/withdrawal index updates.
	upWithComms := &BGPUpdate{
		AnnouncedPrefixes: []string{"192.0.2.0/24"},
		WithdrawnPrefixes: []string{"198.51.100.0/24"},
		ASPath:            []uint32{65001, 203038},
		NextHop:           "192.0.2.1",
		Communities:       []string{"65001:100"},
		LargeCommunities:  []string{"65001:100:1"},
	}
	if err := UpdateBMPRib(ctx, client, "rt1", "192.0.2.1", upWithComms); err != nil {
		t.Fatalf("UpdateBMPRib with communities failed: %v", err)
	}
	if _, _, err := GetBMPPeerRoutesPaginated(ctx, client, "rt1", "192.0.2.1", "bestpath", 0, 10); err != nil {
		t.Fatalf("GetBMPPeerRoutesPaginated bestpath failed: %v", err)
	}
	if commRoutes, err := FindBMPRoutesByCommunity(ctx, client, "rt1", "65001:100"); err != nil || len(commRoutes) == 0 {
		t.Errorf("FindBMPRoutesByCommunity = (%v, %v), want non-empty", commRoutes, err)
	}
	if lcRoutes, err := FindBMPRoutesByLargeCommunity(ctx, client, "rt1", "65001:100:1"); err != nil || len(lcRoutes) == 0 {
		t.Errorf("FindBMPRoutesByLargeCommunity = (%v, %v), want non-empty", lcRoutes, err)
	}
	if asnRoutes, err := FindBMPRoutesByASPath(ctx, client, "rt1", "_65001_203038_"); err != nil || len(asnRoutes) == 0 {
		t.Errorf("FindBMPRoutesByASPath indexed = (%v, %v), want non-empty", asnRoutes, err)
	}
}
