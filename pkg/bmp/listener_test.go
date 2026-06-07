package bmp

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/AS203038/looking-glass/pkg/utils"
	"github.com/redis/go-redis/v9"
)

// TestBMPListenerEndToEnd verifies the BMP listener server handles incoming connection lifecycles and BGP update feeds.
func TestBMPListenerEndToEnd(t *testing.T) {
	redisLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for mock redis: %v", err)
	}
	defer redisLn.Close()

	go func() {
		for {
			conn, err := redisLn.Accept()
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
						} else {
							c.Write([]byte("+OK\r\n"))
						}
					}
				}
			}(conn)
		}
	}()

	client := redis.NewClient(&redis.Options{
		Addr: redisLn.Addr().String(),
	})
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bmpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start BMP listener: %v", err)
	}
	bmpAddr := bmpLn.Addr().String()
	_ = bmpLn.Close()

	rts := utils.RouterMap{
		&utils.RouterInstance{
			Config: &utils.RouterConfig{
				Name:     "test-router",
				Hostname: "127.0.0.1",
			},
		},
	}

	err = StartBMPListener(ctx, bmpAddr, client, rts)
	if err != nil {
		t.Fatalf("StartBMPListener failed: %v", err)
	}

	conn, err := net.Dial("tcp", bmpAddr)
	if err != nil {
		t.Fatalf("failed to dial BMP listener: %v", err)
	}
	defer conn.Close()

	h1 := []byte{3, 0, 0, 0, 48, 3}
	p1 := make([]byte, 42)
	p1[25] = 1
	binary.BigEndian.PutUint32(p1[26:30], 65001)

	_, _ = conn.Write(h1)
	_, _ = conn.Write(p1)

	h2 := []byte{3, 0, 0, 0, 48, 2}
	_, _ = conn.Write(h2)
	_, _ = conn.Write(p1)

	h3 := []byte{3, 0, 0, 0, 71, 0}
	bgpHeader := make([]byte, 19)
	for i := 0; i < 16; i++ {
		bgpHeader[i] = 0xff
	}
	binary.BigEndian.PutUint16(bgpHeader[16:18], 23)
	bgpHeader[18] = 2

	bgpUpdate := []byte{0, 0, 0, 0}

	_, _ = conn.Write(h3)
	_, _ = conn.Write(p1)
	_, _ = conn.Write(bgpHeader)
	_, _ = conn.Write(bgpUpdate)

	hWrongVer := []byte{4, 0, 0, 0, 48, 3}
	_, _ = conn.Write(hWrongVer)

	conn2, err := net.Dial("tcp", bmpAddr)
	if err == nil {
		defer conn2.Close()
		hWrongLen := []byte{3, 0, 0, 0, 2, 3}
		_, _ = conn2.Write(hWrongLen)
	}

	conn3, err := net.Dial("tcp", bmpAddr)
	if err == nil {
		defer conn3.Close()
		hUnknownType := []byte{3, 0, 0, 0, 48, 99}
		_, _ = conn3.Write(hUnknownType)
		_, _ = conn3.Write(p1)
	}

	time.Sleep(10 * time.Millisecond)
}

// TestFindRouterByRemoteIPPorts verifies that findRouterByRemoteIP omits the port number from RouterConfig's Hostname.
func TestFindRouterByRemoteIPPorts(t *testing.T) {
	rts := utils.RouterMap{
		&utils.RouterInstance{
			Config: &utils.RouterConfig{
				Name:     "router-ipv4-port",
				Hostname: "192.0.2.1:12345",
			},
		},
		&utils.RouterInstance{
			Config: &utils.RouterConfig{
				Name:     "router-ipv6-port",
				Hostname: "[2001:db8::1]:12345",
			},
		},
		&utils.RouterInstance{
			Config: &utils.RouterConfig{
				Name:     "router-no-port",
				Hostname: "192.0.2.2",
			},
		},
	}

	tests := []struct {
		name       string
		remoteAddr string
		expected   string
	}{
		{
			name:       "IPv4 remote matches IPv4 with port",
			remoteAddr: "192.0.2.1:54321",
			expected:   "router-ipv4-port",
		},
		{
			name:       "IPv6 remote matches IPv6 with port",
			remoteAddr: "[2001:db8::1]:54321",
			expected:   "router-ipv6-port",
		},
		{
			name:       "IPv4 remote matches IPv4 without port",
			remoteAddr: "192.0.2.2:54321",
			expected:   "router-no-port",
		},
		{
			name:       "No match",
			remoteAddr: "192.0.2.3:54321",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := net.ResolveTCPAddr("tcp", tt.remoteAddr)
			if err != nil {
				t.Fatalf("failed to resolve tcp addr %s: %v", tt.remoteAddr, err)
			}
			got := findRouterByRemoteIP(addr, rts)
			if got != tt.expected {
				t.Errorf("findRouterByRemoteIP(%s) = %q, want %q", tt.remoteAddr, got, tt.expected)
			}
		})
	}
}
