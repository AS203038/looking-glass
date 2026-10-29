package utils

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AS203038/looking-glass/pkg/errs"
	"golang.org/x/crypto/ssh"
)

const certTestUser = "lg"

func newTestSigner(t *testing.T) (ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return signer, priv
}

// writeTestKey writes priv as an OpenSSH PEM private key and returns its path.
func writeTestKey(t *testing.T, dir string, priv ed25519.PrivateKey) string {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	path := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path
}

// writeTestCert signs pub with ca and writes the certificate to path.
func writeTestCert(t *testing.T, path string, ca ssh.Signer, pub ssh.PublicKey, serial, validBefore uint64) {
	t.Helper()
	signTestCert(t, path, ca, newTestCert(pub, serial, validBefore))
}

// newTestCert returns an unsigned user certificate for pub that a test can change before signTestCert.
func newTestCert(pub ssh.PublicKey, serial, validBefore uint64) *ssh.Certificate {
	return &ssh.Certificate{
		Key:             pub,
		Serial:          serial,
		CertType:        ssh.UserCert,
		KeyId:           "looking-glass-test",
		ValidPrincipals: []string{certTestUser},
		ValidBefore:     validBefore,
	}
}

// signTestCert signs cert with ca and writes it to path.
func signTestCert(t *testing.T, path string, ca ssh.Signer, cert *ssh.Certificate) {
	t.Helper()
	if err := cert.SignCert(rand.Reader, ca); err != nil {
		t.Fatalf("sign cert: %v", err)
	}
	if err := os.WriteFile(path, ssh.MarshalAuthorizedKey(cert), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
}

// startCertServer starts an SSH server that accepts only user certificates
// signed by ca and answers every exec request with "ok".
func startCertServer(t *testing.T, ca ssh.PublicKey) string {
	t.Helper()
	checker := &ssh.CertChecker{
		IsUserAuthority: func(auth ssh.PublicKey) bool {
			return bytes.Equal(auth.Marshal(), ca.Marshal())
		},
	}
	config := &ssh.ServerConfig{PublicKeyCallback: checker.Authenticate}
	hostKey, _ := newTestSigner(t)
	config.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveExec(conn, config)
		}
	}()
	return listener.Addr().String()
}

func serveExec(conn net.Conn, config *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer ch.Close()
			for req := range chReqs {
				if req.Type != "exec" {
					req.Reply(false, nil)
					continue
				}
				req.Reply(true, nil)
				ch.Write([]byte("ok"))
				ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				return
			}
		}()
	}
}

type certTestEnv struct {
	ca       ssh.Signer
	key      ssh.Signer
	keyPath  string
	certPath string
	addr     string
}

func newCertTestEnv(t *testing.T) *certTestEnv {
	t.Helper()
	dir := t.TempDir()
	ca, _ := newTestSigner(t)
	key, priv := newTestSigner(t)
	return &certTestEnv{
		ca:       ca,
		key:      key,
		keyPath:  writeTestKey(t, dir, priv),
		certPath: filepath.Join(dir, "id_ed25519-cert.pub"),
		addr:     startCertServer(t, ca.PublicKey()),
	}
}

func (e *certTestEnv) router(cert string) *RouterConfig {
	return &RouterConfig{
		Name:     "cert-test",
		Hostname: e.addr,
		Username: certTestUser,
		SSHKey:   e.keyPath,
		SSHCert:  cert,
	}
}

func TestSSHExecCertLogin(t *testing.T) {
	e := newCertTestEnv(t)
	writeTestCert(t, e.certPath, e.ca, e.key.PublicKey(), 1, ssh.CertTimeInfinity)

	out, err := SSHExec(e.router(e.certPath), []string{"show version"})
	if err != nil {
		t.Fatalf("SSHExec with cert and key: %v", err)
	}
	if len(out) != 1 || out[0] != "ok" {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestSSHExecKeyOnlyRejected(t *testing.T) {
	e := newCertTestEnv(t)

	_, err := SSHExec(e.router(""), []string{"show version"})
	if !errors.Is(err, errs.ConnectionFailed) {
		t.Fatalf("expected errs.ConnectionFailed for a plain key, got %v", err)
	}
}

func TestSSHExecCertKeyMismatch(t *testing.T) {
	e := newCertTestEnv(t)
	other, _ := newTestSigner(t)
	writeTestCert(t, e.certPath, e.ca, other.PublicKey(), 1, ssh.CertTimeInfinity)

	_, err := SSHExec(e.router(e.certPath), []string{"show version"})
	if !errors.Is(err, errs.AuthFailed) {
		t.Fatalf("expected errs.AuthFailed for a cert of another key, got %v", err)
	}
}

func TestSSHExecCertNotACertificate(t *testing.T) {
	e := newCertTestEnv(t)
	if err := os.WriteFile(e.certPath, ssh.MarshalAuthorizedKey(e.key.PublicKey()), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	_, err := SSHExec(e.router(e.certPath), []string{"show version"})
	if !errors.Is(err, errs.AuthFailed) {
		t.Fatalf("expected errs.AuthFailed for a plain public key file, got %v", err)
	}
}

func TestSSHExecHostCertRejected(t *testing.T) {
	e := newCertTestEnv(t)
	cert := newTestCert(e.key.PublicKey(), 1, ssh.CertTimeInfinity)
	cert.CertType = ssh.HostCert
	signTestCert(t, e.certPath, e.ca, cert)

	_, err := SSHExec(e.router(e.certPath), []string{"show version"})
	if !errors.Is(err, errs.AuthFailed) {
		t.Fatalf("expected errs.AuthFailed for a host certificate, got %v", err)
	}
}

// captureSSHLog sends the SSH log records to a JSON buffer for the test.
func captureSSHLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := sshLog
	sshLog = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { sshLog = orig })
	return &buf
}

// sshLogRecord returns the first JSON record in buf with message msg.
func sshLogRecord(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("parse log line %q: %v", line, err)
		}
		if rec["msg"] == msg {
			return rec
		}
	}
	return nil
}

func TestSSHExecCertValidBeforeLogged(t *testing.T) {
	expired := uint64(time.Now().Add(-time.Minute).Unix())
	cases := []struct {
		name        string
		validBefore uint64
		want        string
		warn        bool
	}{
		{"infinity", ssh.CertTimeInfinity, "forever", false},
		{"beyond int64", uint64(math.MaxInt64) + 1, "9223372036854775808", false},
		{"expired", expired, time.Unix(int64(expired), 0).UTC().Format(time.RFC3339), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCertTestEnv(t)
			writeTestCert(t, e.certPath, e.ca, e.key.PublicKey(), 1, tc.validBefore)
			buf := captureSSHLog(t)

			SSHExec(e.router(e.certPath), []string{"show version"})

			loaded := sshLogRecord(t, buf, "ssh cert loaded")
			if loaded == nil {
				t.Fatalf("no \"ssh cert loaded\" record in %s", buf)
			}
			if got := fmt.Sprint(loaded["valid_before"]); !strings.HasPrefix(got, tc.want) {
				t.Fatalf("valid_before = %q, want prefix %q", got, tc.want)
			}
			if sshLogRecord(t, buf, "ssh cert not yet valid") != nil {
				t.Fatalf("unexpected not-yet-valid warning in %s", buf)
			}
			if warned := sshLogRecord(t, buf, "ssh cert expired") != nil; warned != tc.warn {
				t.Fatalf("expired warning = %v, want %v", warned, tc.warn)
			}
		})
	}
}

func TestSSHExecCertNotYetValidLogged(t *testing.T) {
	e := newCertTestEnv(t)
	cert := newTestCert(e.key.PublicKey(), 1, ssh.CertTimeInfinity)
	cert.ValidAfter = uint64(time.Now().Add(time.Hour).Unix())
	signTestCert(t, e.certPath, e.ca, cert)
	buf := captureSSHLog(t)

	if _, err := SSHExec(e.router(e.certPath), []string{"show version"}); !errors.Is(err, errs.ConnectionFailed) {
		t.Fatalf("expected errs.ConnectionFailed for a cert that is not yet valid, got %v", err)
	}
	rec := sshLogRecord(t, buf, "ssh cert not yet valid")
	if rec == nil {
		t.Fatalf("no \"ssh cert not yet valid\" record in %s", buf)
	}
	want := time.Unix(int64(cert.ValidAfter), 0).UTC().Format(time.RFC3339)
	if got := fmt.Sprint(rec["valid_after"]); !strings.HasPrefix(got, want) {
		t.Fatalf("valid_after = %q, want prefix %q", got, want)
	}
	if sshLogRecord(t, buf, "ssh cert expired") != nil {
		t.Fatalf("unexpected expired warning in %s", buf)
	}
}

func TestSSHExecCertRenewedFileIsRead(t *testing.T) {
	e := newCertTestEnv(t)
	expired := uint64(time.Now().Add(-time.Minute).Unix())
	writeTestCert(t, e.certPath, e.ca, e.key.PublicKey(), 1, expired)

	router := e.router(e.certPath)
	if _, err := SSHExec(router, []string{"show version"}); !errors.Is(err, errs.ConnectionFailed) {
		t.Fatalf("expected errs.ConnectionFailed for an expired cert, got %v", err)
	}

	writeTestCert(t, e.certPath, e.ca, e.key.PublicKey(), 2, ssh.CertTimeInfinity)
	if _, err := SSHExec(router, []string{"show version"}); err != nil {
		t.Fatalf("SSHExec after cert renewal: %v", err)
	}
}

func TestParseConfigSSHCertRequiresKey(t *testing.T) {
	dir := t.TempDir()
	parse := func(device string) (*Config, error) {
		t.Helper()
		path := filepath.Join(dir, "config.yaml")
		body := "devices:\n  - name: rt1\n    hostname: 192.0.2.1:22\n" + device
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return ParseConfigYaml(path)
	}

	cfg, err := parse("  - name: no-hostname\n    ssh_cert: /etc/lg/id_ed25519-cert.pub\n")
	if err != nil {
		t.Fatalf("expected a device without hostname to be dropped, not rejected, got %v", err)
	}
	if len(cfg.Devices) != 1 || cfg.Devices[0].Name != "rt1" {
		t.Fatalf("expected only rt1 to remain, got %+v", cfg.Devices)
	}

	_, err = parse("    ssh_cert: /etc/lg/id_ed25519-cert.pub\n")
	if err == nil || !strings.Contains(err.Error(), "ssh_cert requires ssh_key") {
		t.Fatalf("expected ssh_cert without ssh_key to fail validation, got %v", err)
	}

	cfg, err = parse("    ssh_key: /etc/lg/id_ed25519\n    ssh_cert: /etc/lg/id_ed25519-cert.pub\n")
	if err != nil {
		t.Fatalf("expected ssh_cert with ssh_key to pass validation, got %v", err)
	}
	if got := cfg.Devices[0].SSHCert; got != "/etc/lg/id_ed25519-cert.pub" {
		t.Fatalf("ssh_cert not parsed, got %q", got)
	}
}
