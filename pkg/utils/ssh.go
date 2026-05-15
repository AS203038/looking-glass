package utils

import (
	"bytes"
	"log"
	"os"

	"github.com/AS203038/looking-glass/pkg/errs"
	"golang.org/x/crypto/ssh"
)

// sshLogPrefix is prepended to every SSH-related server-side log line
// so operators can easily grep for SSH issues.
const sshLogPrefix = "SSH"

// routerTag returns a short identifier for a router suitable for log lines.
// It avoids leaking credentials and keeps the line compact.
func routerTag(router *RouterConfig) string {
	if router == nil {
		return "router=<nil>"
	}
	return "router=" + router.Name + " host=" + router.Hostname + " user=" + router.Username
}

// SSHExec opens an SSH connection to the configured router and runs each
// command sequentially, returning a slice of stdout strings (one per
// command) in the same order.
//
// Supported authentication methods (offered to the server in this order;
// the server picks whichever matches its policy):
//
//   - publickey       — offered when [RouterConfig.SSHKey] is set.
//   - password        — offered when [RouterConfig.Password] is set.
//   - keyboard-interactive — offered when [RouterConfig.Password] is set;
//     every prompt the server issues is answered with the configured
//     password. This matches the OpenSSH client default and is required
//     by routers/PAM stacks that advertise only `keyboard-interactive`
//     (notably JunOS in some configurations, MikroTik, and Linux hosts
//     with `ChallengeResponseAuthentication yes` / `PasswordAuthentication
//     no`). Multi-prompt MFA flows (e.g. TACACS+ OTP) cannot be satisfied
//     by a static secret; those deployments should use publickey auth.
//
// Error-handling philosophy:
//
//   - Callers (and ultimately RPC clients) only ever see the coarse
//     sentinel errors defined in pkg/errs (AuthFailed, ConnectionFailed,
//     ExecFailed). This prevents information disclosure (hostnames,
//     credentials, internal command text) to external users.
//
//   - The server operator, however, needs the full picture to diagnose
//     issues. Every failure path therefore logs a detailed line via the
//     standard logger (writes to stderr / stdout depending on log
//     configuration) that includes the router identity, the specific
//     stage that failed, the underlying Go error, and — crucially —
//     any stderr captured from the remote session.
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
			// keyboard-interactive: mirror every prompt with the
			// configured password. The server controls prompt text
			// ("Password:", "Verification code:", …); for the common
			// single-prompt case this is exactly the password flow,
			// and for multi-prompt flows there is no better static
			// answer the LG can give.
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
		// Capture stderr separately so we can surface it in the server
		// log when a command exits non-zero or the session blows up.
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
// if truncation occurred. Used to keep log lines bounded for noisy
// router stderr output.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}
