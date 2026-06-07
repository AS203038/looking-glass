package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/AS203038/looking-glass/pkg/errs"
	"golang.org/x/crypto/ssh"
)

var mockRejectSession = false

// TestSSHPool verifies connection pooling liveness, recycle, and fail-fast operations, and tests SSHExec and command execution.
func TestSSHPoolAndExec(t *testing.T) {
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
					if mockRejectSession || newChan.ChannelType() != "session" {
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
								// Parse command payload
								cmdStr := string(req.Payload)
								req.Reply(true, nil)

								if strings.Contains(cmdStr, "fail") {
									// Simulate standard error and exit status 1
									channel.Stderr().Write([]byte("mock exec command failed with error"))
									exitStatus := []byte{0, 0, 0, 1} // exit-status 1
									channel.SendRequest("exit-status", false, exitStatus)
								} else {
									// Simulate standard output success and exit status 0
									channel.Write([]byte("mock-execution-output"))
									exitStatus := []byte{0, 0, 0, 0} // exit-status 0
									channel.SendRequest("exit-status", false, exitStatus)
								}
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

	clientConfig := &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	router := &RouterConfig{
		Name:        "test-router",
		Hostname:    addr,
		Username:    "test",
		Password:    "password",
		SSHPoolSize: 2,
	}

	pool := getPool(router)
	if pool.maxSize != 2 {
		t.Errorf("expected pool maxSize 2, got %v", pool.maxSize)
	}

	// 1. Checkout on empty pool
	client1, err := pool.Checkout(router, clientConfig)
	if err != nil {
		t.Fatalf("failed to checkout client1: %v", err)
	}

	// Verify liveness
	if !isAlive(client1) {
		t.Errorf("expected client1 to be alive")
	}

	// Recycle client1 to pool (success=true)
	pool.Recycle(client1, router, true)

	// 2. Checkout on non-empty pool (reused)
	client2, err := pool.Checkout(router, clientConfig)
	if err != nil {
		t.Fatalf("failed to checkout client2: %v", err)
	}
	if client2 != client1 {
		t.Errorf("expected to reuse the same client connection")
	}

	// 3. Checkout a second connection concurrently (pool maxSize is 2)
	client3, err := pool.Checkout(router, clientConfig)
	if err != nil {
		t.Fatalf("failed to checkout client3: %v", err)
	}

	// 4. Try to checkout a third connection (should fail-fast immediately because maxSize is 2)
	client4, err := pool.Checkout(router, clientConfig)
	if err == nil {
		client4.Close()
		t.Errorf("expected fail-fast error on third checkout when pool is fully utilized")
	} else if !errors.Is(err, errs.PoolExhausted) {
		t.Errorf("expected errs.PoolExhausted, got %v", err)
	}

	// Recycle both connections
	pool.Recycle(client2, router, true)
	pool.Recycle(client3, router, true)

	// Test Recycle with excess connection (pool maxSize is 2, conns has client2 and client3)
	clientExtra, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		t.Fatalf("failed to dial extra client: %v", err)
	}
	pool.Recycle(clientExtra, router, true)

	// Add a dead connection to the pool to test dead-connection checkout filtering (isAlive = false)
	clientDead, err := pool.Checkout(router, clientConfig)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	clientDead.Close() // make it dead
	pool.mu.Lock()
	pool.conns = append(pool.conns, clientDead)
	pool.mu.Unlock()

	// Checkout should detect clientDead, discard it, and dial/return a new alive connection!
	clientAlive, err := pool.Checkout(router, clientConfig)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if clientAlive == clientDead {
		t.Errorf("expected to get a new client connection, not the dead one")
	}
	pool.Recycle(clientAlive, router, true)

	// Recycle with success=false (discard failed connection)
	clientFail, err := pool.Checkout(router, clientConfig)
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	pool.Recycle(clientFail, router, false)

	// Test routerAttrs with nil router
	attrsNil := routerAttrs(nil)
	if len(attrsNil) != 1 || attrsNil[0].Value.String() != "<nil>" {
		t.Errorf("expected <nil> router attribute")
	}

	// 5. Test SSHExec with connection pooling
	out, err := SSHExec(router, []string{"show bgp summary", "show version"})
	if err != nil {
		t.Fatalf("pooled SSHExec failed: %v", err)
	}
	if len(out) != 2 || out[0] != "mock-execution-output" || out[1] != "mock-execution-output" {
		t.Errorf("unexpected output from pooled SSHExec: %v", out)
	}

	// Test SSHExec with empty command list
	outEmpty, err := SSHExec(router, []string{})
	if err != nil || len(outEmpty) != 0 {
		t.Errorf("expected empty output, got %v, err %v", outEmpty, err)
	}

	// 6. Test SSHExec with failed command execution
	_, err = SSHExec(router, []string{"show fail"})
	if !errors.Is(err, errs.ExecFailed) {
		t.Errorf("expected errs.ExecFailed for failed command, got %v", err)
	}

	// 7. Test SSHExec unpooled path
	routerUnpooled := &RouterConfig{
		Name:        "test-router-unpooled",
		Hostname:    addr,
		Username:    "test",
		Password:    "password",
		SSHPoolSize: 0,
	}

	outUnpooled, err := SSHExec(routerUnpooled, []string{"show bgp summary"})
	if err != nil {
		t.Fatalf("unpooled SSHExec failed: %v", err)
	}
	if len(outUnpooled) != 1 || outUnpooled[0] != "mock-execution-output" {
		t.Errorf("unexpected output from unpooled SSHExec: %v", outUnpooled)
	}

	// Test unpooled SSHExec command execution failure
	_, err = SSHExec(routerUnpooled, []string{"show fail"})
	if !errors.Is(err, errs.ExecFailed) {
		t.Errorf("expected errs.ExecFailed for unpooled command failure, got %v", err)
	}

	// 8. Test SSHExec checkout failure on unreachable host
	routerUnreachable := &RouterConfig{
		Name:        "test-router-unreachable",
		Hostname:    "127.0.0.1:1", // closed port
		Username:    "test",
		SSHPoolSize: 2,
	}
	_, err = SSHExec(routerUnreachable, []string{"show version"})
	if !errors.Is(err, errs.ConnectionFailed) {
		t.Errorf("expected errs.ConnectionFailed for unreachable host, got %v", err)
	}

	// 9. Test unpooled SSHExec dial failure on unreachable host
	routerUnpooledUnreachable := &RouterConfig{
		Name:        "test-router-unpooled-unreachable",
		Hostname:    "127.0.0.1:1", // closed port
		Username:    "test",
		SSHPoolSize: 0,
	}
	_, err = SSHExec(routerUnpooledUnreachable, []string{"show version"})
	if !errors.Is(err, errs.ConnectionFailed) {
		t.Errorf("expected errs.ConnectionFailed for unpooled unreachable host, got %v", err)
	}

	// 11. Test SSHExec auth config failure (invalid private key path)
	routerBadKey := &RouterConfig{
		Name:        "test-router-badkey",
		Hostname:    addr,
		Username:    "test",
		SSHKey:      "non-existent-key.pem",
		SSHPoolSize: 0,
	}
	_, err = SSHExec(routerBadKey, []string{"show version"})
	if !errors.Is(err, errs.AuthFailed) {
		t.Errorf("expected errs.AuthFailed (bad key path), got %v", err)
	}

	// 12. Test private key loading happy path
	// Marshal the generated RSA private key to PEM
	privateKeyPEM := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyPEM,
	}
	tmpKeyFile, err := os.CreateTemp("", "lg-test-key-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpKeyFile.Name())

	if err := pem.Encode(tmpKeyFile, pemBlock); err != nil {
		t.Fatal(err)
	}
	tmpKeyFile.Close()

	routerWithKey := &RouterConfig{
		Name:        "test-router-withkey",
		Hostname:    addr,
		Username:    "test",
		SSHKey:      tmpKeyFile.Name(),
		SSHPoolSize: 0,
	}
	outWithKey, err := SSHExec(routerWithKey, []string{"show version"})
	if err != nil {
		t.Fatalf("SSHExec with valid SSHKey failed: %v", err)
	}
	if len(outWithKey) != 1 || outWithKey[0] != "mock-execution-output" {
		t.Errorf("unexpected output from SSHKey SSHExec: %v", outWithKey)
	}

	// 13. Test SSHExec session creation failure (NewSession returns error)
	mockRejectSession = true
	_, err = SSHExec(router, []string{"show version"})
	mockRejectSession = false
	if !errors.Is(err, errs.ExecFailed) {
		t.Errorf("expected errs.ExecFailed for session creation failure, got %v", err)
	}

	// Manually add another pool containing active connections to cover multiple pools in ClosePools
	pool2 := getPool(&RouterConfig{Name: "test-router-pool2", SSHPoolSize: 1})
	clientExtra2, err := ssh.Dial("tcp", addr, clientConfig)
	if err == nil {
		pool2.mu.Lock()
		pool2.conns = append(pool2.conns, clientExtra2)
		pool2.mu.Unlock()
	}

	// Test close pools
	ClosePools()

	p := getPool(router)
	if len(p.conns) != 0 {
		t.Errorf("expected conns to be empty after ClosePools, got %v", len(p.conns))
	}
}

// TestTruncate verifies that the truncate helper function works correctly.
func TestTruncate(t *testing.T) {
	cases := []struct {
		in       string
		n        int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello…(truncated)"},
		{"", 5, ""},
	}

	for _, tc := range cases {
		got := truncate(tc.in, tc.n)
		if got != tc.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.expected)
		}
	}
}
