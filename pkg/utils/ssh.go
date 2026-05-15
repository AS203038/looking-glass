package utils

import (
	"bytes"
	"log"
	"os"

	"github.com/AS203038/looking-glass/pkg/errs"
	"golang.org/x/crypto/ssh"
)

// sshLogPrefix is prepended to every SSH-related server-side log line.
const sshLogPrefix = "SSH"

// routerTag returns a short, credential-free identifier for a router
// suitable for log lines.
func routerTag(router *RouterConfig) string {
	if router == nil {
		return "router=<nil>"
	}
	return "router=" + router.Name + " host=" + router.Hostname + " user=" + router.Username
}

// SSHExec opens an SSH connection to the configured router and runs each
// command sequentially, returning the per-command stdout strings in order.
// Authentication methods offered: publickey (when SSHKey is set), password
// and keyboard-interactive (both when Password is set). Failures are
// returned as the coarse [errs] sentinels; detailed diagnostics are written
// to the server log.
func SSHExec(router *RouterConfig, cmd []string) ([]string, error) {
	auths := []ssh.AuthMethod{}
	if router.SSHKey != "" {
		k, err := os.ReadFile(router.SSHKey)
		if err != nil {
			log.Printf("%s: read ssh key failed (%s key=%s): %v",
				sshLogPrefix, routerTag(router), router.SSHKey, err)
			return nil, errs.AuthFailed
		}
		key, err := ssh.ParsePrivateKey(k)
		if err != nil {
			log.Printf("%s: parse ssh key failed (%s key=%s): %v",
				sshLogPrefix, routerTag(router), router.SSHKey, err)
			return nil, errs.AuthFailed
		}
		auths = append(auths, ssh.PublicKeys(key))
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
	}

	config := &ssh.ClientConfig{
		User:            router.Username,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	client, err := ssh.Dial("tcp", router.Hostname, config)
	if err != nil {
		log.Printf("%s: dial failed (%s): %v",
			sshLogPrefix, routerTag(router), err)
		return nil, errs.ConnectionFailed
	}
	defer client.Close()
	ret := make([]string, len(cmd))
	for i, c := range cmd {
		session, err := client.NewSession()
		if err != nil {
			log.Printf("%s: new session failed (%s cmd_index=%d cmd=%q): %v",
				sshLogPrefix, routerTag(router), i, c, err)
			return nil, errs.ExecFailed
		}
		var stderr bytes.Buffer
		session.Stderr = &stderr
		output, err := session.Output(c)
		if err != nil {
			log.Printf("%s: command failed (%s cmd_index=%d cmd=%q stderr=%q): %v",
				sshLogPrefix, routerTag(router), i, c,
				truncate(stderr.String(), 512), err)
			session.Close()
			return nil, errs.ExecFailed
		}
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
