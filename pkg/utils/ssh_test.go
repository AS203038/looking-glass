package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"testing"

	"github.com/AS203038/looking-glass/pkg/errs"
	"golang.org/x/crypto/ssh"
)

func TestSSHPool(t *testing.T) {
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
					go func() {
						defer ch.Close()
						for req := range reqs {
							req.Reply(true, nil)
						}
					}()
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

	// Test close pools
	ClosePools()

	p := getPool(router)
	if len(p.conns) != 0 {
		t.Errorf("expected conns to be empty after ClosePools, got %v", len(p.conns))
	}
}
