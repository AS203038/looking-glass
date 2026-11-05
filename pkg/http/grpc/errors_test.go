package grpc

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/utils"
	"golang.org/x/crypto/ssh"
)

type mockTimeoutRouter struct{ mockGrpcRouter }

func (mockTimeoutRouter) Traceroute(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return nil, fmt.Errorf("rpc to 198.51.100.7:830: %w", errs.ExecTimeout)
}

type mockInternalErrorRouter struct{ mockGrpcRouter }

func (mockInternalErrorRouter) Traceroute(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return nil, fmt.Errorf("dial tcp 198.51.100.7:22: %w", errs.ConnectionFailed)
}

type mockCanceledRouter struct{ mockGrpcRouter }

func (mockCanceledRouter) Traceroute(*utils.RouterConfig, *utils.IPNet) ([]string, error) {
	return nil, errs.ExecCanceled
}

// TestMuxErrorsHaveCodeAndMessage sends RPCs over HTTP to the mounted service and
// verifies that each failure reaches the client with its code and the sentinel text only.
func TestMuxErrorsHaveCodeAndMessage(t *testing.T) {
	rts := utils.RouterMap{}
	for i, r := range []utils.Router{mockTimeoutRouter{}, mockInternalErrorRouter{}, mockPoolExhaustedRouter{}, mockCanceledRouter{}} {
		rts = append(rts, &utils.RouterInstance{
			Config:      &utils.RouterConfig{Name: fmt.Sprintf("router%d", i+1)},
			Router:      r,
			HealthCheck: &utils.HealthCheck{Healthy: true},
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	mux := http.NewServeMux()
	done := Mux(ctx, mux, rts)
	t.Cleanup(func() { cancel(); <-done })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name, body, code, message string
		status                    int
	}{
		{"timeout", `{"routerId":1,"target":"192.0.2.1"}`, "deadline_exceeded", "router did not answer in time", http.StatusGatewayTimeout},
		{"empty target", `{"routerId":1,"target":""}`, "invalid_argument", "invalid IP", http.StatusBadRequest},
		{"unknown router", `{"routerId":9,"target":"192.0.2.1"}`, "not_found", "router unknown", http.StatusNotFound},
		{"internal error", `{"routerId":2,"target":"192.0.2.1"}`, "unavailable", "execution error", http.StatusServiceUnavailable},
		{"pool exhausted", `{"routerId":3,"target":"192.0.2.1"}`, "resource_exhausted", "ssh pool exhausted", http.StatusTooManyRequests},
		{"canceled", `{"routerId":4,"target":"192.0.2.1"}`, "canceled", "request canceled", 499},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/lookingglass.v0.LookingGlassService/Traceroute", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			var got struct{ Code, Message string }
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("body %q: %v", raw, err)
			}
			if resp.StatusCode != tc.status || got.Code != tc.code || got.Message != tc.message {
				t.Errorf("got %d %q %q, want %d %q %q", resp.StatusCode, got.Code, got.Message, tc.status, tc.code, tc.message)
			}
			if strings.Contains(string(raw), "198.51.100.7") {
				t.Errorf("body %q has the internal error text", raw)
			}
		})
	}
}

// startHangingSSHServer starts an SSH server that answers each command except
// traceroute, which it never answers. It sends on hung when a traceroute arrives.
func startHangingSSHServer(t *testing.T, hung chan<- struct{}) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
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
					ch, chReqs, err := newChan.Accept()
					if err != nil {
						continue
					}
					go func() {
						for req := range chReqs {
							req.Reply(true, nil)
							if req.Type != "exec" {
								continue
							}
							if strings.Contains(string(req.Payload), "traceroute") {
								hung <- struct{}{}
								continue
							}
							ch.Write([]byte("output\n"))
							ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
							ch.Close()
						}
					}()
				}
			}()
		}
	}()
	return listener.Addr().String()
}

// TestMuxClientDisconnectFreesPoolSlot sends a request over HTTP that hangs on the router, ends
// the HTTP request, and verifies that the next request does not fail with resource_exhausted.
func TestMuxClientDisconnectFreesPoolSlot(t *testing.T) {
	hung := make(chan struct{}, 1)
	rts := utils.RouterMap{&utils.RouterInstance{
		Config:      &utils.RouterConfig{Name: "hang@" + t.Name(), Hostname: startHangingSSHServer(t, hung), Username: "test", SSHPoolSize: 1},
		Router:      mockGrpcRouter{},
		HealthCheck: &utils.HealthCheck{Healthy: true},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	mux := http.NewServeMux()
	done := Mux(ctx, mux, rts)
	t.Cleanup(func() { cancel(); <-done })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	post := func(ctx context.Context, method, body string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/lookingglass.v0.LookingGlassService/"+method, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return srv.Client().Do(req)
	}
	reqCtx, disconnect := context.WithCancel(context.Background())
	defer disconnect()
	first := make(chan error, 1)
	go func() {
		_, err := post(reqCtx, "Traceroute", `{"routerId":1,"target":"192.0.2.1"}`)
		first <- err
	}()
	select {
	case <-hung:
	case <-time.After(5 * time.Second):
		t.Fatal("the router got no traceroute")
	}
	disconnect()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first request: err = %v, want context.Canceled", err)
	}

	var status int
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		resp, err := post(context.Background(), "Ping", `{"routerId":1,"target":"192.0.2.1"}`)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if status = resp.StatusCode; status == http.StatusOK {
			return
		}
	}
	t.Errorf("request after the disconnect: status %d, want %d", status, http.StatusOK)
}

// TestPublicErrorsCoverEveryErrsSentinel verifies that each error variable in pkg/errs has a code in publicErrors.
func TestPublicErrorsCoverEveryErrsSentinel(t *testing.T) {
	files, err := filepath.Glob("../../errs/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("pkg/errs sources: %v %v", files, err)
	}
	mapped := map[string]bool{}
	for _, p := range publicErrors {
		mapped[p.err.Error()] = true
	}
	for _, f := range files {
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "New" {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && !mapped[strings.Trim(lit.Value, `"`)] {
				t.Errorf("%s: errors.New(%s) has no entry in publicErrors", filepath.Base(f), lit.Value)
			}
			return true
		})
	}
}
