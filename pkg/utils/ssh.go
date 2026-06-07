package utils

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/logging"
	"golang.org/x/crypto/ssh"
)

// sshLog is the component-tagged logger for SSH-related events.
var sshLog = logging.Component("ssh")

// routerAttrs returns the slog attributes describing a router for log lines.
func routerAttrs(router *RouterConfig) []slog.Attr {
	if router == nil {
		return []slog.Attr{slog.String("router", "<nil>")}
	}
	return []slog.Attr{
		slog.String("router", router.Name),
		slog.String("host", router.Hostname),
		slog.String("user", router.Username),
	}
}

// sshLogAt emits a structured record at lvl for an SSH-side event.
func sshLogAt(lvl slog.Level, msg string, router *RouterConfig, extra ...slog.Attr) {
	attrs := routerAttrs(router)
	attrs = append(attrs, extra...)
	sshLog.LogAttrs(context.Background(), lvl, msg, attrs...)
}

type sshPool struct {
	mu      sync.Mutex
	conns   []*ssh.Client
	maxSize int
	numOpen int
}

func isAlive(client *ssh.Client) bool {
	_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
	return err == nil
}

func (p *sshPool) Checkout(router *RouterConfig, config *ssh.ClientConfig) (*ssh.Client, error) {
	for {
		p.mu.Lock()
		if len(p.conns) > 0 {
			idx := len(p.conns) - 1
			client := p.conns[idx]
			p.conns = p.conns[:idx]
			p.mu.Unlock()

			if isAlive(client) {
				return client, nil
			}
			client.Close()

			p.mu.Lock()
			p.numOpen--
			p.mu.Unlock()
			sshLogAt(slog.LevelDebug, "discarded dead pooled connection on checkout", router)
			continue
		}

		if p.numOpen >= p.maxSize {
			p.mu.Unlock()
			sshLogAt(slog.LevelWarn, "pool fully utilized, fail-fast triggered", router, slog.Int("pool_size", p.maxSize))
			return nil, errs.PoolExhausted
		}

		p.numOpen++
		p.mu.Unlock()

		sshLogAt(slog.LevelDebug, "ssh pool slot acquired, dialing new connection", router)
		client, err := ssh.Dial("tcp", router.Hostname, config)
		if err != nil {
			p.mu.Lock()
			p.numOpen--
			p.mu.Unlock()
			return nil, err
		}
		return client, nil
	}
}

func (p *sshPool) Recycle(client *ssh.Client, router *RouterConfig, success bool) {
	if client == nil {
		return
	}

	if !success || !isAlive(client) {
		client.Close()
		p.mu.Lock()
		p.numOpen--
		p.mu.Unlock()
		sshLogAt(slog.LevelDebug, "closed connection on recycle (dead or failed)", router)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.conns) < p.maxSize {
		p.conns = append(p.conns, client)
		sshLogAt(slog.LevelDebug, "recycled connection to pool", router, slog.Int("idle_conns", len(p.conns)))
	} else {
		client.Close()
		p.numOpen--
		sshLogAt(slog.LevelDebug, "closed connection (pool full)", router)
	}
}

var (
	poolsMu sync.Mutex
	pools   = make(map[string]*sshPool)
)

func getPool(router *RouterConfig) *sshPool {
	poolsMu.Lock()
	defer poolsMu.Unlock()

	pool, ok := pools[router.Name]
	if !ok {
		pool = &sshPool{
			maxSize: router.SSHPoolSize,
		}
		pools[router.Name] = pool
	}
	return pool
}

// ClosePools closes all idle connections across all configured pools.
func ClosePools() {
	poolsMu.Lock()
	defer poolsMu.Unlock()

	for name, pool := range pools {
		pool.mu.Lock()
		closedCount := 0
		for _, client := range pool.conns {
			client.Close()
			closedCount++
			pool.numOpen--
		}
		sshLog.Debug("closed ssh pool", slog.String("router", name), slog.Int("closed_conns", closedCount))
		pool.conns = nil
		pool.mu.Unlock()
	}
}

// execOnClient runs the command sequence sequentially over newly instantiated
// sessions on the provided active client connection.
func execOnClient(client *ssh.Client, router *RouterConfig, cmd []string) ([]string, error) {
	ret := make([]string, len(cmd))
	for i, c := range cmd {
		sshLogAt(slog.LevelDebug, "ssh exec start", router,
			slog.Int("cmd_index", i),
			slog.String("cmd", c))
		execStart := time.Now()
		session, err := client.NewSession()
		if err != nil {
			sshLogAt(slog.LevelError, "new session failed", router,
				slog.Int("cmd_index", i),
				slog.String("cmd", c),
				slog.Any("err", err))
			return nil, errs.ExecFailed
		}
		var stderr bytes.Buffer
		session.Stderr = &stderr
		output, err := session.Output(c)
		if err != nil {
			sshLogAt(slog.LevelError, "command failed", router,
				slog.Int("cmd_index", i),
				slog.String("cmd", c),
				slog.Duration("duration", time.Since(execStart)),
				slog.String("stderr", truncate(stderr.String(), 512)),
				slog.Any("err", err))
			session.Close()
			return nil, errs.ExecFailed
		}
		sshLogAt(slog.LevelDebug, "ssh exec ok", router,
			slog.Int("cmd_index", i),
			slog.Duration("duration", time.Since(execStart)),
			slog.Int("output_bytes", len(output)))
		ret[i] = string(output)
		session.Close()
	}
	return ret, nil
}

// SSHExec opens an SSH connection to the configured router and runs each
// command sequentially, returning the per-command stdout strings in order.
// Authentication methods offered: publickey (when SSHKey is set), password
// and keyboard-interactive (both when Password is set). Failures are
// returned as the coarse [errs] sentinels; detailed diagnostics are written
// to the server log.
func SSHExec(router *RouterConfig, cmd []string) ([]string, error) {
	if !router.HasSSHCredentials() {
		return nil, errs.AuthFailed
	}
	auths := []ssh.AuthMethod{}
	methods := make([]string, 0, 3)
	if router.SSHKey != "" {
		k, err := os.ReadFile(router.SSHKey)
		if err != nil {
			sshLogAt(slog.LevelError, "read ssh key failed", router,
				slog.String("key", router.SSHKey),
				slog.Any("err", err))
			return nil, errs.AuthFailed
		}
		key, err := ssh.ParsePrivateKey(k)
		if err != nil {
			sshLogAt(slog.LevelError, "parse ssh key failed", router,
				slog.String("key", router.SSHKey),
				slog.Any("err", err))
			return nil, errs.AuthFailed
		}
		auths = append(auths, ssh.PublicKeys(key))
		methods = append(methods, "publickey")
	}
	if router.Password != "" {
		auths = append(auths,
			ssh.Password(router.Password),
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = router.Password
				}
				return answers, nil
			}),
		)
		methods = append(methods, "password", "keyboard-interactive")
	}

	config := &ssh.ClientConfig{
		User:            router.Username,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	if router.SSHPoolSize <= 0 {
		sshLogAt(slog.LevelDebug, "ssh dial start (unpooled)", router,
			slog.Any("auth_methods", methods),
			slog.Int("commands", len(cmd)))
		dialStart := time.Now()
		client, err := ssh.Dial("tcp", router.Hostname, config)
		if err != nil {
			sshLogAt(slog.LevelError, "dial failed", router,
				slog.Duration("duration", time.Since(dialStart)),
				slog.Any("err", err))
			return nil, errs.ConnectionFailed
		}
		sshLogAt(slog.LevelDebug, "ssh dial ok (unpooled)", router,
			slog.Duration("duration", time.Since(dialStart)))
		defer func() {
			client.Close()
			sshLogAt(slog.LevelDebug, "ssh client close (unpooled)", router)
		}()
		return execOnClient(client, router, cmd)
	}

	// Pooled Execution Path
	pool := getPool(router)
	client, err := pool.Checkout(router, config)
	if err != nil {
		if errors.Is(err, errs.PoolExhausted) {
			return nil, errs.PoolExhausted
		}
		sshLogAt(slog.LevelError, "checkout failed", router, slog.Any("err", err))
		return nil, errs.ConnectionFailed
	}

	ret, err := execOnClient(client, router, cmd)
	pool.Recycle(client, router, err == nil)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

// truncate shortens s to at most n bytes, appending an ellipsis marker
// when truncation occurs.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}
