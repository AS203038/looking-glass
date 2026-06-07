package bmp

import (
	"context"
	"fmt"
	"net"
	"strings"
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
