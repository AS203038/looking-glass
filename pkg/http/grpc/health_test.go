package grpc

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestHealthCoordination verifies the Redis-coordination functions for background health checking.
func TestHealthCoordination(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock redis server: %v", err)
	}
	defer ln.Close()

	mockStateJSON := `{"healthy":true,"checked":"2026-06-05T08:00:00Z","source":"mock-peer"}`

	// RESP server for health checking
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
					lowerReq := strings.ToLower(req)

					if strings.Contains(lowerReq, "setinfo") {
						c.Write([]byte("+OK\r\n+OK\r\n"))
					} else if strings.Contains(lowerReq, "setnx") {
						// Return integer 1 (success) for SETNX lease acquire
						c.Write([]byte(":1\r\n"))
					} else if strings.Contains(lowerReq, "get") {
						if strings.Contains(lowerReq, "non-existent") {
							c.Write([]byte("$-1\r\n"))
						} else {
							// Return bulk string containing JSON
							c.Write([]byte("$" + strconv.Itoa(len(mockStateJSON)) + "\r\n" + mockStateJSON + "\r\n"))
						}
					} else if strings.Contains(lowerReq, "set") {
						c.Write([]byte("+OK\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						c.Write([]byte("%7\r\n$6\r\nserver\r\n$5\r\nredis\r\n$7\r\nversion\r\n$5\r\n7.0.0\r\n$5\r\nproto\r\n:2\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"))
					} else {
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

	// Set client
	SetRedis(client)

	ctx := context.Background()

	// 1. Try acquire lease (success path)
	ok, err := tryAcquireHealthLease(ctx, "router1")
	if err != nil {
		t.Errorf("failed to acquire lease: %v", err)
	}
	if !ok {
		t.Errorf("expected lease acquire to succeed")
	}

	// 2. Write state
	err = writeHealthState(ctx, "router1", healthState{
		Healthy: true,
		Checked: time.Now(),
		Source:  "test-node",
	})
	if err != nil {
		t.Errorf("writeHealthState failed: %v", err)
	}

	// 3. Read state (existent)
	state, ok, err := readHealthState(ctx, "router1")
	if err != nil {
		t.Errorf("readHealthState failed: %v", err)
	}
	if !ok {
		t.Errorf("expected state to exist")
	}
	if !state.Healthy || state.Source != "mock-peer" {
		t.Errorf("unexpected health state parsed: %+v", state)
	}

	// 4. Read state (non-existent)
	_, ok, err = readHealthState(ctx, "non-existent-router")
	if err != nil {
		t.Errorf("readHealthState non-existent failed: %v", err)
	}
	if ok {
		t.Errorf("expected state to not exist")
	}

	// 5. Test with nil client
	SetRedis(nil)
	ok, err = tryAcquireHealthLease(ctx, "router1")
	if err != nil || ok {
		t.Errorf("tryAcquireHealthLease nil bypass failed")
	}
	_, ok, _ = readHealthState(ctx, "router1")
	if ok {
		t.Errorf("readHealthState nil bypass failed")
	}
	err = writeHealthState(ctx, "router1", healthState{})
	if err != nil {
		t.Errorf("writeHealthState nil bypass failed: %v", err)
	}
}
