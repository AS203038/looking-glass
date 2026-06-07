package grpc

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

// TestRPCCacheKeys verifies that the caching key generation is stable and correctly prefix-namespaced.
func TestRPCCacheKeys(t *testing.T) {
	// Stable hash verification
	h1 := rpcCacheHash("my-aspath-pattern")
	h2 := rpcCacheHash("my-aspath-pattern")
	if h1 != h2 {
		t.Errorf("rpcCacheHash is non-deterministic: %s vs %s", h1, h2)
	}

	expectedPrefix := rpcCacheKeyPrefix()

	cases := []struct {
		name     string
		key      string
		contains string
	}{
		{"ping", pingCacheKey(1, "1.1.1.1"), expectedPrefix + "ping:1:1.1.1.1"},
		{"traceroute", tracerouteCacheKey(2, "2.2.2.2"), expectedPrefix + "traceroute:2:2.2.2.2"},
		{"bgpsummary", bgpSummaryCacheKey(3), expectedPrefix + "bgpsummary:3"},
		{"bgproute", bgpRouteCacheKey(4, "8.8.8.8"), expectedPrefix + "bgproute:4:8.8.8.8"},
		{"bgpcommunity", bgpCommunityCacheKey(5, "65000:100"), expectedPrefix + "bgpcommunity:5:65000:100"},
		{"bgplargecommunity", bgpLargeCommunityCacheKey(6, "65000:100:200"), expectedPrefix + "bgplargecommunity:6:65000:100:200"},
		{"bgpaspath", bgpASPathCacheKey(7, "_65000$"), expectedPrefix + "bgpaspath:7:" + rpcCacheHash("_65000$")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.key != tc.contains {
				t.Errorf("expected key %q, got %q", tc.contains, tc.key)
			}
		})
	}
}

// TestNilRPCCache verifies that cache operations gracefully bypass and return misses when unconfigured.
func TestNilRPCCache(t *testing.T) {
	SetRPCCache(nil, 5*time.Minute)

	ctx := context.Background()
	msg := &pb.BGPSummaryResponse{}

	// get should return false (miss)
	if rpcCacheGet(ctx, "some-key", msg) {
		t.Errorf("rpcCacheGet returned true on nil client")
	}

	// set should be a safe no-op
	rpcCacheSet(ctx, "some-key", msg)
}

// TestLiveRPCCache verifies actual cache GET and SET operations against a mock in-memory Redis server.
func TestLiveRPCCache(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock redis server: %v", err)
	}
	defer ln.Close()

	// Setup mock marshaled payload to return on a cache HIT
	sampleResp := &pb.BGPSummaryResponse{
		Result:     []byte("mock-bgp-summary"),
		ParserKind: pb.ParserKind_PARSER_KIND_NATIVE_JSON,
	}
	marshaled, _ := proto.Marshal(sampleResp)

	// Simple mock RESP protocol server
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
					t.Logf("MOCK REDIS RECEIVED: %q", req)
					lowerReq := strings.ToLower(req)
					if strings.Contains(lowerReq, "setinfo") {
						// Replies to both CLIENT SETINFO commands sent back-to-back in a single packet
						c.Write([]byte("+OK\r\n+OK\r\n"))
					} else if strings.Contains(lowerReq, "get") {
						if strings.Contains(lowerReq, "non-existent") {
							// Cache MISS
							c.Write([]byte("$-1\r\n"))
						} else {
							// Cache HIT: return raw bulk string bytes
							c.Write([]byte("$" + strconv.Itoa(len(marshaled)) + "\r\n" + string(marshaled) + "\r\n"))
						}
					} else if strings.Contains(lowerReq, "set") {
						c.Write([]byte("+OK\r\n"))
					} else if strings.Contains(lowerReq, "ping") {
						c.Write([]byte("+PONG\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						// Handshake response for HELLO command (returns a map reply)
						c.Write([]byte("%7\r\n$6\r\nserver\r\n$5\r\nredis\r\n$7\r\nversion\r\n$5\r\n7.0.0\r\n$5\r\nproto\r\n:2\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"))
					} else {
						// Fallback response to other handshake commands like CLIENT
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	client := redis.NewClient(&redis.Options{
		Addr:     ln.Addr().String(),
		Protocol: 2,
	})
	defer client.Close()

	SetRPCCache(client, 5*time.Minute)

	ctx := context.Background()

	// 1. Test cache MIS
	destMiss := &pb.BGPSummaryResponse{}
	if rpcCacheGet(ctx, "non-existent-key", destMiss) {
		t.Errorf("expected cache miss for non-existent-key")
	}

	// 2. Test cache HIT
	destHit := &pb.BGPSummaryResponse{}
	rawBytes, getErr := client.Get(ctx, "existent-key").Bytes()
	if getErr != nil {
		t.Errorf("DEBUG direct client.Get error: %v", getErr)
	} else {
		t.Logf("DEBUG direct client.Get raw bytes: %v", rawBytes)
	}

	if !rpcCacheGet(ctx, "existent-key", destHit) {
		t.Errorf("expected cache hit for existent-key")
	}

	if string(destHit.GetResult()) != "mock-bgp-summary" {
		t.Errorf("expected cached result 'mock-bgp-summary', got %q", destHit.GetResult())
	}

	if destHit.GetParserKind() != pb.ParserKind_PARSER_KIND_NATIVE_JSON {
		t.Errorf("expected cached parser kind NATIVE_JSON, got %v", destHit.GetParserKind())
	}

	// 3. Test cache STORE
	rpcCacheSet(ctx, "new-key", sampleResp)
}
