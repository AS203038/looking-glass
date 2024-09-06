package utils

import (
	"os"

	"github.com/AS203038/looking-glass/pkg/errs"
	"golang.org/x/crypto/ssh"
)

func SSHExec(router *RouterConfig, cmd []string) ([]string, error) {
	auths := []ssh.AuthMethod{ssh.Password(router.Password)}
	if router.SSHKey != "" {
		k, err := os.ReadFile(router.SSHKey)
		if err != nil {
			return nil, errs.AuthFailed
		}
		key, err := ssh.ParsePrivateKey(k)
		if err != nil {
			return nil, errs.AuthFailed
		}
		auths = append(auths, ssh.PublicKeys(key))
	}
	config := &ssh.ClientConfig{
		User:            router.Username,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	client, err := ssh.Dial("tcp", router.Hostname, config)
	if err != nil {
		return nil, errs.ConnectionFailed
	}
	defer client.Close()
	ret := make([]string, len(cmd))
	for i, c := range cmd {
		session, err := client.NewSession()
		if err != nil {
			return nil, errs.ExecFailed
		}
		output, err := session.Output(c)
		if err != nil {
			return nil, errs.ExecFailed
		}
		ret[i] = string(output)
		session.Close()
	}
	return ret, nil
}
