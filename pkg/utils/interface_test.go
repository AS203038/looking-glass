package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

type mockRouter struct{}

func (mockRouter) Ping(*RouterConfig, *IPNet) ([]string, error) { return []string{"ping"}, nil }
func (mockRouter) Traceroute(*RouterConfig, *IPNet) ([]string, error) { return []string{"traceroute"}, nil }
func (mockRouter) BGPSummary(*RouterConfig) ([]string, error) { return []string{"bgp.summary"}, nil }
func (mockRouter) BGPRoute(*RouterConfig, *IPNet) ([]string, error) { return []string{"bgp.route"}, nil }
func (mockRouter) BGPCommunity(*RouterConfig, string) ([]string, error) { return []string{"bgp.community"}, nil }
func (mockRouter) BGPLargeCommunity(*RouterConfig, string) ([]string, error) { return []string{"bgp.largecommunity"}, nil }
func (mockRouter) BGPASPath(*RouterConfig, string) ([]string, error) { return []string{"bgp.aspath"}, nil }
func (mockRouter) BGPPeerRoutes(*RouterConfig, string, string, string) ([]string, error) { return []string{"bgp.peer_routes"}, nil }

type mockRouterError struct{}

func (mockRouterError) Ping(*RouterConfig, *IPNet) ([]string, error) {
	return nil, errors.New("mock ping err")
}
func (mockRouterError) Traceroute(*RouterConfig, *IPNet) ([]string, error) {
	return nil, errors.New("mock traceroute err")
}
func (mockRouterError) BGPSummary(*RouterConfig) ([]string, error) {
	return nil, errors.New("mock summary err")
}
func (mockRouterError) BGPRoute(*RouterConfig, *IPNet) ([]string, error) {
	return nil, errors.New("mock route err")
}
func (mockRouterError) BGPCommunity(*RouterConfig, string) ([]string, error) {
	return nil, errors.New("mock community err")
}
func (mockRouterError) BGPLargeCommunity(*RouterConfig, string) ([]string, error) {
	return nil, errors.New("mock large community err")
}
func (mockRouterError) BGPASPath(*RouterConfig, string) ([]string, error) {
	return nil, errors.New("mock aspath err")
}
func (mockRouterError) BGPPeerRoutes(*RouterConfig, string, string, string) ([]string, error) {
	return nil, errors.New("mock peer routes err")
}

// TestRouterInstanceWrappers verifies that the RouterInstance execution wrapper methods connect, execute, and return output correctly.
func TestRouterInstanceWrappers(t *testing.T) {
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

	ri := &RouterInstance{
		Config: &RouterConfig{
			Name:        "test-router",
			Hostname:    addr,
			Username:    "test",
			SSHPoolSize: 0, // unpooled execution path
		},
		Router:      mockRouter{},
		HealthCheck: &HealthCheck{},
	}

	ip, _ := NewIPNET("1.1.1.1")

	// 1. Verify wrappers execute commands successfully on the mock server
	if err := ri.Healthcheck(); err != nil {
		t.Errorf("Healthcheck failed: %v", err)
	}
	if !ri.HealthCheck.Healthy {
		t.Errorf("expected healthy to be true")
	}

	if _, err := ri.Ping(ip); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
	if _, err := ri.Traceroute(ip); err != nil {
		t.Errorf("Traceroute failed: %v", err)
	}
	if _, err := ri.BGPSummary(); err != nil {
		t.Errorf("BGPSummary failed: %v", err)
	}
	if _, err := ri.BGPRoute(ip); err != nil {
		t.Errorf("BGPRoute failed: %v", err)
	}
	if _, err := ri.BGPCommunity("65000:100"); err != nil {
		t.Errorf("BGPCommunity failed: %v", err)
	}
	if _, err := ri.BGPLargeCommunity("65000:100:200"); err != nil {
		t.Errorf("BGPLargeCommunity failed: %v", err)
	}
	if _, err := ri.BGPASPath("_65000$"); err != nil {
		t.Errorf("BGPASPath failed: %v", err)
	}
	if _, err := ri.BGPPeerRoutes("192.0.2.1", "peer_1", "received"); err != nil {
		t.Errorf("BGPPeerRoutes failed: %v", err)
	}

	// 2. Test wrappers fail-path validation (using unreachable host)
	riFail := &RouterInstance{
		Config: &RouterConfig{
			Name:        "test-router-fail",
			Hostname:    "127.0.0.1:1", // closed port
			Username:    "test",
			SSHPoolSize: 0,
		},
		Router:      mockRouter{},
		HealthCheck: &HealthCheck{},
	}

	if err := riFail.Healthcheck(); err == nil {
		t.Errorf("expected Healthcheck to fail")
	}
	if riFail.HealthCheck.Healthy {
		t.Errorf("expected healthy to be false on fail")
	}

	if _, err := riFail.Ping(ip); err == nil {
		t.Errorf("expected Ping to fail")
	}
	if _, err := riFail.Traceroute(ip); err == nil {
		t.Errorf("expected Traceroute to fail")
	}
	if _, err := riFail.BGPSummary(); err == nil {
		t.Errorf("expected BGPSummary to fail")
	}
	if _, err := riFail.BGPRoute(ip); err == nil {
		t.Errorf("expected BGPRoute to fail")
	}
	if _, err := riFail.BGPCommunity("65000:100"); err == nil {
		t.Errorf("expected BGPCommunity to fail")
	}
	if _, err := riFail.BGPLargeCommunity("65000:100:200"); err == nil {
		t.Errorf("expected BGPLargeCommunity to fail")
	}
	if _, err := riFail.BGPASPath("_65000$"); err == nil {
		t.Errorf("expected BGPASPath to fail")
	}
	if _, err := riFail.BGPPeerRoutes("192.0.2.1", "peer_1", "received"); err == nil {
		t.Errorf("expected BGPPeerRoutes to fail")
	}

	// 3. Test wrappers where the Router methods themselves fail (mockRouterError)
	riRouterFail := &RouterInstance{
		Config: &RouterConfig{
			Name:        "test-router-routerfail",
			Hostname:    addr,
			Username:    "test",
			SSHPoolSize: 0,
		},
		Router:      mockRouterError{},
		HealthCheck: &HealthCheck{},
	}

	if _, err := riRouterFail.Ping(ip); err == nil {
		t.Errorf("expected Ping error")
	}
	if _, err := riRouterFail.Traceroute(ip); err == nil {
		t.Errorf("expected Traceroute error")
	}
	if _, err := riRouterFail.BGPSummary(); err == nil {
		t.Errorf("expected BGPSummary error")
	}
	if _, err := riRouterFail.BGPRoute(ip); err == nil {
		t.Errorf("expected BGPRoute error")
	}
	if _, err := riRouterFail.BGPCommunity("65000:100"); err == nil {
		t.Errorf("expected BGPCommunity error")
	}
	if _, err := riRouterFail.BGPLargeCommunity("65000:100:200"); err == nil {
		t.Errorf("expected BGPLargeCommunity error")
	}
	if _, err := riRouterFail.BGPASPath("_65000$"); err == nil {
		t.Errorf("expected BGPASPath error")
	}
	if _, err := riRouterFail.BGPPeerRoutes("192.0.2.1", "peer_1", "received"); err == nil {
		t.Errorf("expected BGPPeerRoutes error")
	}
}

// TestRouterMap verifies RouterMap lookup logic.
func TestRouterMap(t *testing.T) {
	ri1 := &RouterInstance{Config: &RouterConfig{Name: "r1"}}
	ri2 := &RouterInstance{Config: &RouterConfig{Name: "r2"}}
	rm := RouterMap{ri1, ri2}

	// 1. Lookup by Name
	got, ok := rm.Get("r2")
	if !ok || got != ri2 {
		t.Errorf("RouterMap.Get failed")
	}

	_, ok = rm.Get("nonexistent")
	if ok {
		t.Errorf("RouterMap.Get returned true for nonexistent name")
	}

	// 2. Lookup by ID (1-based public identifier)
	got, ok = rm.GetByID(1)
	if !ok || got != ri1 {
		t.Errorf("RouterMap.GetByID(1) failed")
	}

	got, ok = rm.GetByID(2)
	if !ok || got != ri2 {
		t.Errorf("RouterMap.GetByID(2) failed")
	}

	_, ok = rm.GetByID(0)
	if ok {
		t.Errorf("RouterMap.GetByID(0) should return false")
	}

	_, ok = rm.GetByID(3)
	if ok {
		t.Errorf("RouterMap.GetByID(3) should return false")
	}
}
