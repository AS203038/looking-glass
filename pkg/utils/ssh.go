package utils

import (
	"bytes"
	"context"
	"log/slog"
	"os"
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

// SSHExec opens an SSH connection to the configured router and runs each
// command sequentially, returning the per-command stdout strings in order.
// Authentication methods offered: publickey (when SSHKey is set), password
// and keyboard-interactive (both when Password is set). Failures are
// returned as the coarse [errs] sentinels; detailed diagnostics are written
// to the server log.
func SSHExec(router *RouterConfig, cmd []string) ([]string, error) {
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
	sshLogAt(slog.LevelDebug, "ssh dial start", router,
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
	sshLogAt(slog.LevelDebug, "ssh dial ok", router,
		slog.Duration("duration", time.Since(dialStart)))
	defer func() {
		client.Close()
		sshLogAt(slog.LevelDebug, "ssh client close", router)
	}()
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

// truncate shortens s to at most n bytes, appending an ellipsis marker
// when truncation occurs.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}
