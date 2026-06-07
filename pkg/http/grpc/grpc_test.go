package grpc

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/redis/go-redis/v9"
)

// TestMuxAndHealthcheck verifies Mux mounting and healthcheck loop behavior.
func TestMuxAndHealthcheck(t *testing.T) {
	mux := http.NewServeMux()
	rts := utils.RouterMap{
		{
			Config:      &utils.RouterConfig{Name: "router1"},
			HealthCheck: &utils.HealthCheck{Healthy: false},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	Mux(ctx, mux, rts)
	time.Sleep(5 * time.Millisecond)

	ctx2, cancel2 := context.WithCancel(context.Background())
	healthcheckInterval = 2 * time.Millisecond
	defer func() {
		healthcheckInterval = time.Minute
	}()

	mux2 := http.NewServeMux()
	Mux(ctx2, mux2, rts)
	time.Sleep(10 * time.Millisecond)
	cancel2()
	time.Sleep(5 * time.Millisecond)
}

// TestProbeRouterOnce verifies the probeRouterOnce reachability state machine.
func TestProbeRouterOnce(t *testing.T) {
	log := slog.Default()

	r := &utils.RouterInstance{
		Config:      &utils.RouterConfig{Name: "router1"},
		HealthCheck: &utils.HealthCheck{Healthy: false},
	}

	probeRouterOnce(context.Background(), log, r)
	if r.HealthCheck.Healthy {
		t.Errorf("expected router to be unhealthy on SSH failure when Redis is nil")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock RESP server: %v", err)
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
					lowerReq := strings.ToLower(req)

					if strings.Contains(lowerReq, "setinfo") {
						c.Write([]byte("+OK\r\n+OK\r\n"))
					} else if strings.Contains(lowerReq, "setnx") {
						c.Write([]byte(":1\r\n"))
					} else if strings.Contains(lowerReq, "hlen") {
						c.Write([]byte(":1\r\n"))
					} else if strings.Contains(lowerReq, "get") {
						c.Write([]byte("$-1\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						c.Write([]byte("%1\r\n$5\r\nproto\r\n:2\r\n"))
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

	SetRedis(client)
	defer SetRedis(nil)

	r2 := &utils.RouterInstance{
		Config:      &utils.RouterConfig{Name: "router2"},
		HealthCheck: &utils.HealthCheck{Healthy: false},
	}

	probeRouterOnce(context.Background(), log, r2)
	if !r2.HealthCheck.Healthy {
		t.Errorf("expected router to be healthy due to active BMP override")
	}

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock RESP server 2: %v", err)
	}
	defer ln2.Close()

	mockStateJSON := `{"healthy":true,"checked":"2026-06-05T08:00:00Z","source":"peer-node"}`

	go func() {
		for {
			conn, err := ln2.Accept()
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
					} else if strings.Contains(lowerReq, "lg:health:lease:") {
						c.Write([]byte("$-1\r\n"))
					} else if strings.Contains(lowerReq, "hlen") {
						c.Write([]byte(":0\r\n"))
					} else if strings.Contains(lowerReq, "get") {
						c.Write([]byte("$" + strconv.Itoa(len(mockStateJSON)) + "\r\n" + mockStateJSON + "\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						c.Write([]byte("%1\r\n$5\r\nproto\r\n:2\r\n"))
					} else {
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	client2 := redis.NewClient(&redis.Options{
		Addr:     ln2.Addr().String(),
		Protocol: 2,
	})
	defer client2.Close()

	SetRedis(client2)

	r3 := &utils.RouterInstance{
		Config:      &utils.RouterConfig{Name: "router3"},
		HealthCheck: &utils.HealthCheck{Healthy: false},
	}

	probeRouterOnce(context.Background(), log, r3)
	if !r3.HealthCheck.Healthy {
		t.Errorf("expected router to adopt peer health check state and be healthy")
	}

	ln3, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock RESP server 3: %v", err)
	}
	defer ln3.Close()

	go func() {
		for {
			conn, err := ln3.Accept()
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
					} else if strings.Contains(lowerReq, "lg:health:lease:") {
						c.Write([]byte("$-1\r\n"))
					} else if strings.Contains(lowerReq, "hlen") {
						c.Write([]byte(":0\r\n"))
					} else if strings.Contains(lowerReq, "get") {
						c.Write([]byte("-ERR mock redis error\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						c.Write([]byte("%1\r\n$5\r\nproto\r\n:2\r\n"))
					} else {
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	client3 := redis.NewClient(&redis.Options{
		Addr:     ln3.Addr().String(),
		Protocol: 2,
	})
	defer client3.Close()

	SetRedis(client3)

	r4 := &utils.RouterInstance{
		Config:      &utils.RouterConfig{Name: "router4"},
		HealthCheck: &utils.HealthCheck{Healthy: true},
	}

	probeRouterOnce(context.Background(), log, r4)
	if r4.HealthCheck.Healthy {
		t.Errorf("expected router to fall back to local probe (unhealthy) on state read failure")
	}

	ln4, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock RESP server 4: %v", err)
	}
	defer ln4.Close()

	go func() {
		for {
			conn, err := ln4.Accept()
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
					} else if strings.Contains(lowerReq, "lg:health:lease:") {
						c.Write([]byte("$-1\r\n"))
					} else if strings.Contains(lowerReq, "hlen") {
						c.Write([]byte(":0\r\n"))
					} else if strings.Contains(lowerReq, "get") {
						c.Write([]byte("$-1\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						c.Write([]byte("%1\r\n$5\r\nproto\r\n:2\r\n"))
					} else {
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	client4 := redis.NewClient(&redis.Options{
		Addr:     ln4.Addr().String(),
		Protocol: 2,
	})
	defer client4.Close()

	SetRedis(client4)

	r5 := &utils.RouterInstance{
		Config:      &utils.RouterConfig{Name: "router5"},
		HealthCheck: &utils.HealthCheck{Healthy: false},
	}

	probeRouterOnce(context.Background(), log, r5)
	if r5.HealthCheck.Healthy {
		t.Errorf("expected router to skip state transition and remain unhealthy when peer state is not yet published")
	}

	r5.HealthCheck.Healthy = false
	SetRedis(nil)
}

// TestProbeRouterOnceNoCredentials verifies healthcheck when SSH credentials are not configured.
func TestProbeRouterOnceNoCredentials(t *testing.T) {
	log := slog.Default()

	// Setup a mock RESP server that returns count > 0 for hlen
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock RESP server: %v", err)
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
					lowerReq := strings.ToLower(req)

					if strings.Contains(lowerReq, "setinfo") {
						c.Write([]byte("+OK\r\n+OK\r\n"))
					} else if strings.Contains(lowerReq, "hlen") {
						c.Write([]byte(":1\r\n")) // HLEN > 0 (BMP Active)
					} else if strings.Contains(lowerReq, "get") {
						c.Write([]byte("$-1\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						c.Write([]byte("%1\r\n$5\r\nproto\r\n:2\r\n"))
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

	SetRedis(client)
	defer SetRedis(nil)

	// Case 1: No SSH credentials, BMP is active (hlen > 0). Router should be healthy.
	rActive := &utils.RouterInstance{
		Config:      &utils.RouterConfig{Name: "router_active"}, // has no username/password/sshkey
		HealthCheck: &utils.HealthCheck{Healthy: false},
	}
	probeRouterOnce(context.Background(), log, rActive)
	if !rActive.HealthCheck.Healthy {
		t.Errorf("expected router to be healthy when SSH credentials are not configured but BMP is active")
	}

	// Case 2: No SSH credentials, BMP is inactive.
	// Let's stop the previous mock server and start one where hlen returns 0
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start second mock RESP server: %v", err)
	}
	defer ln2.Close()

	go func() {
		for {
			conn, err := ln2.Accept()
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
					} else if strings.Contains(lowerReq, "hlen") {
						c.Write([]byte(":0\r\n")) // HLEN = 0 (BMP Inactive)
					} else if strings.Contains(lowerReq, "get") {
						c.Write([]byte("$-1\r\n"))
					} else if strings.Contains(lowerReq, "hello") {
						c.Write([]byte("%1\r\n$5\r\nproto\r\n:2\r\n"))
					} else {
						c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	client2 := redis.NewClient(&redis.Options{
		Addr:     ln2.Addr().String(),
		Protocol: 2,
	})
	defer client2.Close()

	SetRedis(client2)

	rInactive := &utils.RouterInstance{
		Config:      &utils.RouterConfig{Name: "router_inactive"},
		HealthCheck: &utils.HealthCheck{Healthy: true},
	}
	probeRouterOnce(context.Background(), log, rInactive)
	if rInactive.HealthCheck.Healthy {
		t.Errorf("expected router to be unhealthy when SSH credentials are not configured and BMP is inactive")
	}
}
