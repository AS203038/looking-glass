package utils

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AS203038/looking-glass/pkg/errs"
	"golang.org/x/crypto/ssh"
	"nemith.io/netconf"
	ncssh "nemith.io/netconf/transport/ssh"
)

const (
	netconfTestBase10 = "urn:ietf:params:netconf:base:1.0"
	netconfTestBase11 = "urn:ietf:params:netconf:base:1.1"
	netconfTestEOM    = "]]>]]>"

	// netconfTestSROSRPC is the SR OS MD-CLI raw command action.
	netconfTestSROSRPC = `<action xmlns="urn:ietf:params:xml:ns:yang:1"><global-operations xmlns="urn:nokia.com:sros:ns:yang:sr:oper-global"><md-cli-raw-command><md-cli-input-line>{{.Command}}</md-cli-input-line></md-cli-raw-command></global-operations></action>`
	// netconfTestJunosRPC is the Junos text command RPC.
	netconfTestJunosRPC = `<command format="text">{{.Command}}</command>`
	// netconfTestPartial is the reply body that makes the server send only the start of a reply.
	netconfTestPartial = "\x00partial"
)

// netconfTestServer is an in-process SSH server with a "netconf" subsystem.
// It answers SR OS md-cli-raw-command and Junos command RPCs.
type netconfTestServer struct {
	addr string
	// base11 makes the server offer base:1.1, so the session uses chunked framing.
	base11 bool
	// noSubsystem makes the server refuse the netconf subsystem.
	noSubsystem bool
	// noHello makes the server accept the subsystem and never send its hello.
	noHello bool
	// stallAfterClose makes the server stop answering on a connection after
	// close-session, like SR OS, which drops the SSH connection then.
	stallAfterClose bool
	// partialClose makes the server send only the start of its close-session reply.
	partialClose bool

	mu       sync.Mutex
	conns    int
	sessions int
	commands []string
}

func startNetconfServer(t *testing.T, base11, noSubsystem bool) *netconfTestServer {
	t.Helper()
	return startNetconfTestServer(t, &netconfTestServer{base11: base11, noSubsystem: noSubsystem})
}

// startNetconfTestServer starts s, configured by its option fields.
func startNetconfTestServer(t *testing.T, s *netconfTestServer) *netconfTestServer {
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

	s.addr = listener.Addr().String()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns++
			s.mu.Unlock()
			go s.serveConn(conn, config)
		}
	}()
	return s
}

// stallConn discards all data it reads after stall is called.
type stallConn struct {
	net.Conn
	once    sync.Once
	stalled chan struct{}
}

func (c *stallConn) stall() { c.once.Do(func() { close(c.stalled) }) }

func (c *stallConn) Read(p []byte) (int, error) {
	for {
		n, err := c.Conn.Read(p)
		select {
		case <-c.stalled:
			if err != nil {
				return 0, err
			}
		default:
			return n, err
		}
	}
}

func (s *netconfTestServer) serveConn(conn net.Conn, config *ssh.ServerConfig) {
	sc := &stallConn{Conn: conn, stalled: make(chan struct{})}
	_, chans, reqs, err := ssh.NewServerConn(sc, config)
	if err != nil {
		return
	}
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
				ok := req.Type == "subsystem" && !s.noSubsystem &&
					bytes.HasSuffix(req.Payload, []byte("netconf"))
				req.Reply(ok, nil)
				if ok {
					go ssh.DiscardRequests(chReqs)
					s.serveNetconf(ch, sc.stall)
					return
				}
			}
		}()
	}
}

// netconfTestRPC holds the parts of a client <rpc> the server acts on.
type netconfTestRPC struct {
	MessageID    string    `xml:"message-id,attr"`
	CloseSession *struct{} `xml:"close-session"`
	SROSInput    *string   `xml:"action>global-operations>md-cli-raw-command>md-cli-input-line"`
	JunosCommand *string   `xml:"command"`
}

func (s *netconfTestServer) serveNetconf(ch ssh.Channel, stall func()) {
	r := bufio.NewReader(ch)
	s.mu.Lock()
	s.sessions++
	id := s.sessions
	s.mu.Unlock()

	if s.noHello {
		io.Copy(io.Discard, ch)
		return
	}

	caps := "<capability>" + netconfTestBase10 + "</capability>"
	if s.base11 {
		caps += "<capability>" + netconfTestBase11 + "</capability>"
	}
	fmt.Fprintf(ch, `<?xml version="1.0" encoding="UTF-8"?><hello xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><capabilities>%s</capabilities><session-id>%d</session-id></hello>%s`, caps, id, netconfTestEOM)
	if _, err := netconfTestReadEOM(r); err != nil {
		return
	}

	for {
		var msg []byte
		var err error
		if s.base11 {
			msg, err = netconfTestReadChunked(r)
		} else {
			msg, err = netconfTestReadEOM(r)
		}
		if err != nil {
			return
		}
		var rpc netconfTestRPC
		if err := xml.Unmarshal(msg, &rpc); err != nil {
			return
		}
		var body string
		switch {
		case rpc.CloseSession != nil:
			if s.partialClose {
				s.writePartial(ch, rpc.MessageID)
				return
			}
			s.write(ch, rpc.MessageID, "<ok/>")
			if s.stallAfterClose {
				stall()
			}
			return
		case rpc.SROSInput != nil:
			body = s.reply(*rpc.SROSInput, true)
		case rpc.JunosCommand != nil:
			body = s.reply(*rpc.JunosCommand, false)
		default:
			body = "<rpc-error><error-type>protocol</error-type><error-tag>unknown-element</error-tag><error-severity>error</error-severity></rpc-error>"
		}
		if body == "" {
			// Hang: never reply, wait for the client to close the channel.
			continue
		}
		if body == netconfTestPartial {
			s.writePartial(ch, rpc.MessageID)
			return
		}
		s.write(ch, rpc.MessageID, body)
	}
}

// reply records cmd and builds the reply body. A command that contains
// "error" gets an rpc-error, "hang" gets no reply, "partial" gets only the
// start of a reply, "nooutput" gets <ok/> and
// "busy" gets the SR OS reply for a command the router stopped and "twomessages"
// gets two error-message elements.
func (s *netconfTestServer) reply(cmd string, sros bool) string {
	s.mu.Lock()
	s.commands = append(s.commands, cmd)
	s.mu.Unlock()

	var esc strings.Builder
	xml.EscapeText(&esc, []byte(cmd))
	switch {
	case strings.Contains(cmd, "error"):
		return "<rpc-error><error-type>application</error-type><error-tag>invalid-value</error-tag><error-severity>error</error-severity><error-message>MINOR: CLI Invalid command</error-message></rpc-error>"
	case strings.Contains(cmd, "hang"):
		return ""
	case strings.Contains(cmd, "partial"):
		return netconfTestPartial
	case strings.Contains(cmd, "nooutput"):
		return "<ok/>"
	case sros && strings.Contains(cmd, "twomessages"):
		return `<results><md-cli-output-block></md-cli-output-block></results><error-message>MINOR: CLI first</error-message><error-message>MINOR: CLI second</error-message>`
	case sros && strings.Contains(cmd, "busy"):
		return `<nokiaoper:results xmlns:nokiaoper="urn:nokia.com:sros:ns:yang:sr:oper-global">` +
			`<nokiaoper:md-cli-output-block>` + "\nBGP IPv4 Routes\n" + `</nokiaoper:md-cli-output-block>` +
			`</nokiaoper:results>` +
			`<nokiaoper:status xmlns:nokiaoper="urn:nokia.com:sros:ns:yang:sr:oper-global">terminated-incomplete</nokiaoper:status>` +
			`<nokiaoper:error-message xmlns:nokiaoper="urn:nokia.com:sros:ns:yang:sr:oper-global">MINOR: CLI #2005: BGP routing daemon is busy</nokiaoper:error-message>`
	case sros:
		return `<nokiaoper:operation-id xmlns:nokiaoper="urn:nokia.com:sros:ns:yang:sr:oper-global">1</nokiaoper:operation-id>` +
			`<nokiaoper:start-time xmlns:nokiaoper="urn:nokia.com:sros:ns:yang:sr:oper-global">2026-01-01T00:00:00.0Z</nokiaoper:start-time>` +
			`<nokiaoper:results xmlns:nokiaoper="urn:nokia.com:sros:ns:yang:sr:oper-global">` +
			`<nokiaoper:md-cli-output-block>` + "\nout: " + `</nokiaoper:md-cli-output-block>` +
			`<nokiaoper:md-cli-output-block>` + esc.String() + "\n" + `</nokiaoper:md-cli-output-block>` +
			`</nokiaoper:results>`
	default:
		return "<output>\njunos: " + esc.String() + "\n</output>"
	}
}

func (s *netconfTestServer) write(ch ssh.Channel, msgID, body string) {
	msg := `<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="` + msgID + `">` + body + `</rpc-reply>`
	if s.base11 {
		fmt.Fprintf(ch, "\n#%d\n%s\n##\n", len(msg), msg)
		return
	}
	io.WriteString(ch, msg+netconfTestEOM)
}

// writePartial sends the start of a reply and then waits for the client to
// close the channel. With base:1.1 the start is one complete chunk.
func (s *netconfTestServer) writePartial(ch ssh.Channel, msgID string) {
	msg := `<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="` + msgID + `"><ok`
	if s.base11 {
		fmt.Fprintf(ch, "\n#%d\n%s", len(msg), msg)
	} else {
		io.WriteString(ch, msg)
	}
	io.Copy(io.Discard, ch)
}

func (s *netconfTestServer) stats() (conns, sessions int, commands []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns, s.sessions, append([]string(nil), s.commands...)
}

// netconfTestReadEOM reads one base:1.0 message up to the end-of-message marker.
func netconfTestReadEOM(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf = append(buf, b)
		if bytes.HasSuffix(buf, []byte(netconfTestEOM)) {
			return buf[:len(buf)-len(netconfTestEOM)], nil
		}
	}
}

// netconfTestReadChunked reads one base:1.1 chunked message (RFC 6242 section 4.2).
func netconfTestReadChunked(r *bufio.Reader) ([]byte, error) {
	var msg []byte
	for {
		head := make([]byte, 2)
		if _, err := io.ReadFull(r, head); err != nil {
			return nil, err
		}
		if string(head) != "\n#" {
			return nil, fmt.Errorf("bad chunk header %q", head)
		}
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "#" {
			return msg, nil
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		chunk := make([]byte, n)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return nil, err
		}
		msg = append(msg, chunk...)
	}
}

// netconfTestRouter returns a router config. Pools are global and keyed by
// name, so the name includes the server address.
func netconfTestRouter(name, addr string, poolSize int) *RouterConfig {
	return &RouterConfig{
		Name:        name + "@" + addr,
		Hostname:    addr,
		Username:    "test",
		SSHPoolSize: poolSize,
	}
}

func netconfTestSpec(t *testing.T, rpc, output string) *NetconfSpec {
	t.Helper()
	spec, err := NewNetconfSpec(rpc, output, "")
	if err != nil {
		t.Fatalf("NewNetconfSpec: %v", err)
	}
	return spec
}

// TestNetconfExecSROS verifies two commands over base:1.1 chunked framing:
// order is kept and the text of all prefixed output elements is joined.
func TestNetconfExecSROS(t *testing.T) {
	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")

	out, err := NetconfExec(netconfTestRouter("nc-sros", srv.addr, 0), spec,
		[]string{"show router bgp summary", "show router route-table"})
	if err != nil {
		t.Fatalf("NetconfExec: %v", err)
	}
	want := []string{"\nout: show router bgp summary\n", "\nout: show router route-table\n"}
	if len(out) != 2 || out[0] != want[0] || out[1] != want[1] {
		t.Errorf("output = %q, want %q", out, want)
	}
	_, sessions, commands := srv.stats()
	if sessions != 1 {
		t.Errorf("sessions = %d, want 1 (one NETCONF session per request)", sessions)
	}
	if len(commands) != 2 || commands[0] != "show router bgp summary" || commands[1] != "show router route-table" {
		t.Errorf("server commands = %q", commands)
	}
}

// TestNetconfExecBase10 verifies end-of-message framing with a base:1.0 server
// and a Junos style text command RPC.
func TestNetconfExecBase10(t *testing.T) {
	srv := startNetconfServer(t, false, false)
	spec := netconfTestSpec(t, netconfTestJunosRPC, "output")

	out, err := NetconfExec(netconfTestRouter("nc-junos", srv.addr, 0), spec, []string{"show bgp summary"})
	if err != nil {
		t.Fatalf("NetconfExec: %v", err)
	}
	if len(out) != 1 || out[0] != "\njunos: show bgp summary\n" {
		t.Errorf("output = %q", out)
	}
}

// TestNetconfExecEscapesCommand verifies that XML special characters in a
// command are escaped on the wire and reach the server unchanged.
func TestNetconfExecEscapesCommand(t *testing.T) {
	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	cmd := `show router bgp routes aspath-regex "<&>" '_1$'`

	out, err := NetconfExec(netconfTestRouter("nc-escape", srv.addr, 0), spec, []string{cmd})
	if err != nil {
		t.Fatalf("NetconfExec: %v", err)
	}
	if len(out) != 1 || out[0] != "\nout: "+cmd+"\n" {
		t.Errorf("output = %q", out)
	}
	_, _, commands := srv.stats()
	if len(commands) != 1 || commands[0] != cmd {
		t.Errorf("server saw %q, want %q", commands, cmd)
	}
}

// TestNetconfExecCommandError verifies that the text of the configured error
// element follows the output, as the router CLI shows it.
func TestNetconfExecCommandError(t *testing.T) {
	srv := startNetconfServer(t, true, false)
	spec, err := NewNetconfSpec(netconfTestSROSRPC, "md-cli-output-block", "error-message")
	if err != nil {
		t.Fatalf("NewNetconfSpec: %v", err)
	}

	out, err := NetconfExec(netconfTestRouter("nc-busy", srv.addr, 0), spec,
		[]string{"show busy", "show version", "show twomessages"})
	if err != nil {
		t.Fatalf("NetconfExec: %v", err)
	}
	want := []string{"\nBGP IPv4 Routes\nMINOR: CLI #2005: BGP routing daemon is busy\n", "\nout: show version\n", "MINOR: CLI first\nMINOR: CLI second\n"}
	if len(out) != 3 || out[0] != want[0] || out[1] != want[1] || out[2] != want[2] {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestNetconfExecRPCError verifies that an rpc-error maps to errs.ExecFailed.
func TestNetconfExecRPCError(t *testing.T) {
	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")

	_, err := NetconfExec(netconfTestRouter("nc-rpcerror", srv.addr, 0), spec,
		[]string{"show version", "show error"})
	if !errors.Is(err, errs.ExecFailed) {
		t.Errorf("err = %v, want errs.ExecFailed", err)
	}
}

// TestNetconfExecMissingOutput verifies that a reply without the configured
// output element maps to errs.ExecFailed.
func TestNetconfExecMissingOutput(t *testing.T) {
	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")

	_, err := NetconfExec(netconfTestRouter("nc-nooutput", srv.addr, 0), spec, []string{"show nooutput"})
	if !errors.Is(err, errs.ExecFailed) {
		t.Errorf("err = %v, want errs.ExecFailed", err)
	}
}

// TestNetconfExecTimeout verifies the per-RPC timeout and that the pooled
// client is discarded after it.
func TestNetconfExecTimeout(t *testing.T) {
	old := netconfTimeout
	netconfTimeout = 300 * time.Millisecond
	t.Cleanup(func() { netconfTimeout = old })

	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	router := netconfTestRouter("nc-timeout", srv.addr, 1)

	start := time.Now()
	_, err := NetconfExec(router, spec, []string{"show hang"})
	if !errors.Is(err, errs.ExecFailed) {
		t.Errorf("err = %v, want errs.ExecFailed", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("NetconfExec took %v, the timeout did not apply", d)
	}

	pool := getPool(router)
	pool.mu.Lock()
	idle, open := len(pool.conns), pool.numOpen
	pool.mu.Unlock()
	if idle != 0 || open != 0 {
		t.Errorf("pool idle=%d open=%d after timeout, want the client discarded", idle, open)
	}

	// The next request dials a new connection and works.
	out, err := NetconfExec(router, spec, []string{"show version"})
	if err != nil || len(out) != 1 {
		t.Errorf("NetconfExec after timeout: out=%q err=%v", out, err)
	}
	if conns, _, _ := srv.stats(); conns != 2 {
		t.Errorf("conns = %d, want 2", conns)
	}
}

// netconfExecWithin runs NetconfExec and fails the test when it does not return within d.
func netconfExecWithin(t *testing.T, d time.Duration, router *RouterConfig, spec *NetconfSpec, cmd []string) ([]string, error) {
	t.Helper()
	type result struct {
		out []string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := NetconfExec(router, spec, cmd)
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-time.After(d):
		t.Fatalf("NetconfExec did not return within %v", d)
		return nil, nil
	}
}

// TestNetconfExecPooled verifies that the pool limits connections but a NETCONF
// request never reuses one: SR OS drops the SSH connection after close-session.
func TestNetconfExecPooled(t *testing.T) {
	srv := startNetconfTestServer(t, &netconfTestServer{base11: true, stallAfterClose: true})
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	router := netconfTestRouter("nc-pooled", srv.addr, 1)

	for i := range 2 {
		if _, err := netconfExecWithin(t, 10*time.Second, router, spec, []string{"show version"}); err != nil {
			t.Fatalf("NetconfExec %d: %v", i, err)
		}
	}
	conns, sessions, _ := srv.stats()
	if conns != 2 || sessions != 2 {
		t.Errorf("conns = %d, sessions = %d, want 2 and 2 (one connection per request)", conns, sessions)
	}
	pool := getPool(router)
	pool.mu.Lock()
	idle, open := len(pool.conns), pool.numOpen
	pool.mu.Unlock()
	if idle != 0 || open != 0 {
		t.Errorf("pool idle=%d open=%d, want 0 and 0", idle, open)
	}
}

// TestNetconfExecSkipsIdleConnection verifies that NETCONF dials a new connection
// when the pool holds an idle one, for example from SSHExec to the same device.
func TestNetconfExecSkipsIdleConnection(t *testing.T) {
	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	router := netconfTestRouter("nc-idle", srv.addr, 2)

	idle, err := ssh.Dial("tcp", srv.addr, &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey()})
	if err != nil {
		t.Fatal(err)
	}
	pool := getPool(router)
	pool.mu.Lock()
	pool.numOpen++
	pool.mu.Unlock()
	pool.Recycle(idle, router, true)

	if _, err := netconfExecWithin(t, 10*time.Second, router, spec, []string{"show version"}); err != nil {
		t.Fatalf("NetconfExec: %v", err)
	}
	if conns, sessions, _ := srv.stats(); conns != 2 || sessions != 1 {
		t.Errorf("conns = %d, sessions = %d, want 2 and 1 (a new connection for NETCONF)", conns, sessions)
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.conns) != 1 || pool.conns[0] != idle || pool.numOpen != 1 {
		t.Errorf("pool idle=%d open=%d, want the planted idle connection only", len(pool.conns), pool.numOpen)
	}
}

// TestNetconfExecPoolExhausted verifies that a request and the health check fail at once with
// errs.PoolExhausted while another NETCONF session holds the only pool slot.
func TestNetconfExecPoolExhausted(t *testing.T) {
	old := netconfTimeout
	netconfTimeout = 2 * time.Second
	t.Cleanup(func() { netconfTimeout = old })

	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	ri := &RouterInstance{
		Config:      netconfTestRouter("nc-exhausted", srv.addr, 1),
		Router:      netconfMockRouter{spec: spec},
		HealthCheck: &HealthCheck{Healthy: true},
	}

	held := make(chan error, 1)
	go func() {
		_, err := NetconfExec(ri.Config, spec, []string{"show hang"})
		held <- err
	}()
	pool := getPool(ri.Config)
	deadline := time.Now().Add(time.Second)
	for {
		pool.mu.Lock()
		open := pool.numOpen
		pool.mu.Unlock()
		if open == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the first request did not take the pool slot")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if _, err := netconfExecWithin(t, 500*time.Millisecond, ri.Config, spec, []string{"show version"}); !errors.Is(err, errs.PoolExhausted) {
		t.Errorf("second request: err = %v, want errs.PoolExhausted", err)
	}
	if err := ri.Healthcheck(); !errors.Is(err, errs.PoolExhausted) || ri.HealthCheck.Healthy {
		t.Errorf("Healthcheck: err=%v healthy=%v, want errs.PoolExhausted and unhealthy", err, ri.HealthCheck.Healthy)
	}
	if err := <-held; !errors.Is(err, errs.ExecFailed) {
		t.Errorf("held request: err = %v, want errs.ExecFailed after the timeout", err)
	}
}

// TestNetconfExecPartialReply verifies that the RPC timeout also bounds the
// read of a reply that stops after its start, and frees the pool slot.
func TestNetconfExecPartialReply(t *testing.T) {
	old := netconfTimeout
	netconfTimeout = 300 * time.Millisecond
	t.Cleanup(func() { netconfTimeout = old })

	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	router := netconfTestRouter("nc-partial", srv.addr, 1)

	for i := range 2 {
		if _, err := netconfExecWithin(t, 10*time.Second, router, spec, []string{"show partial"}); !errors.Is(err, errs.ExecFailed) {
			t.Errorf("NetconfExec %d: err = %v, want errs.ExecFailed (not a held pool slot)", i, err)
		}
	}
	if _, _, commands := srv.stats(); len(commands) != 2 {
		t.Errorf("server got commands %q, want the partial reply twice (not a setup timeout)", commands)
	}
}

// TestNetconfCommandAfterExpiredDeadline verifies that no RPC is sent after a
// deadline closed the client, even when the RPC before it succeeded.
func TestNetconfCommandAfterExpiredDeadline(t *testing.T) {
	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	router := netconfTestRouter("nc-expired", srv.addr, 1)

	_, err := withSSHClient(router, 1, false, func(client *ssh.Client) ([]string, error) {
		tr, err := ncssh.NewTransport(client)
		if err != nil {
			return nil, err
		}
		sess, err := netconf.NewSession(tr)
		if err != nil {
			return nil, err
		}
		defer client.Close()
		dl := &netconfDeadlines{client: client}
		dl.expired.Store(true)
		_, err = netconfCommand(dl, sess, router, spec, 0, "show version")
		return nil, err
	})
	if !errors.Is(err, errs.ExecFailed) {
		t.Errorf("err = %v, want errs.ExecFailed", err)
	}
	if _, _, commands := srv.stats(); len(commands) != 0 {
		t.Errorf("server got commands %q after the deadline, want none", commands)
	}
}

// TestNetconfExecPartialCloseReply verifies that the close-session timeout also
// bounds the read of a close-session reply that stops after its start.
func TestNetconfExecPartialCloseReply(t *testing.T) {
	old := netconfCloseTimeout
	netconfCloseTimeout = 300 * time.Millisecond
	t.Cleanup(func() { netconfCloseTimeout = old })

	srv := startNetconfTestServer(t, &netconfTestServer{base11: true, partialClose: true})
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	router := netconfTestRouter("nc-partialclose", srv.addr, 1)

	out, err := netconfExecWithin(t, 10*time.Second, router, spec, []string{"show version"})
	if err != nil || len(out) != 1 {
		t.Errorf("NetconfExec: out=%q err=%v, want the output (a failed close is only logged)", out, err)
	}
}

// TestNetconfExecNoHello verifies that a server that never sends its hello
// fails the request within the timeout and frees the pool slot.
func TestNetconfExecNoHello(t *testing.T) {
	old := netconfTimeout
	netconfTimeout = 300 * time.Millisecond
	t.Cleanup(func() { netconfTimeout = old })

	srv := startNetconfTestServer(t, &netconfTestServer{base11: true, noHello: true})
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	router := netconfTestRouter("nc-nohello", srv.addr, 1)

	for i := range 2 {
		if _, err := netconfExecWithin(t, 10*time.Second, router, spec, []string{"show version"}); !errors.Is(err, errs.ExecFailed) {
			t.Errorf("NetconfExec %d: err = %v, want errs.ExecFailed (not a held pool slot)", i, err)
		}
	}
}

// TestNetconfExecNoSubsystem verifies that a device without a netconf
// subsystem maps to errs.ExecFailed.
func TestNetconfExecNoSubsystem(t *testing.T) {
	srv := startNetconfServer(t, true, true)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")

	_, err := NetconfExec(netconfTestRouter("nc-nosubsystem", srv.addr, 0), spec, []string{"show version"})
	if !errors.Is(err, errs.ExecFailed) {
		t.Errorf("err = %v, want errs.ExecFailed", err)
	}
}

// TestNetconfExecErrors verifies the connection and credential sentinels.
func TestNetconfExecErrors(t *testing.T) {
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")

	_, err := NetconfExec(netconfTestRouter("nc-unreachable", "127.0.0.1:1", 0), spec, []string{"show version"})
	if !errors.Is(err, errs.ConnectionFailed) {
		t.Errorf("unreachable: err = %v, want errs.ConnectionFailed", err)
	}
	_, err = NetconfExec(&RouterConfig{Name: "nc-nouser", Hostname: "127.0.0.1:1"}, spec, []string{"show version"})
	if !errors.Is(err, errs.AuthFailed) {
		t.Errorf("no username: err = %v, want errs.AuthFailed", err)
	}
}

// TestNewNetconfSpec verifies the template checks done at load time.
func TestNewNetconfSpec(t *testing.T) {
	cases := []struct {
		name    string
		rpc     string
		output  string
		wantErr string
	}{
		{"sros", netconfTestSROSRPC, "md-cli-output-block", ""},
		{"junos", netconfTestJunosRPC, "output", ""},
		{"missing rpc", "", "output", "netconf.rpc is required"},
		{"missing output", netconfTestJunosRPC, "", "netconf.output is required"},
		{"bad template", `<command>{{.Command</command>`, "output", "netconf.rpc"},
		{"unknown field", `<command>{{.Cmd}}</command>`, "output", "netconf.rpc"},
		{"unclosed element", `<command>{{.Command}}`, "output", "well-formed"},
		{"mismatched element", `<command>{{.Command}}</cmd>`, "output", "well-formed"},
		{"text only", `{{.Command}}`, "output", "no XML element"},
		{"command unused", `<get-config><source><running/></source></get-config>`, "output", "{{.Command}}"},
		{"hard-coded sample command", `<command>show &#34;&lt;&amp;&gt;&#34; &#39;x&#39;</command>`, "output", "{{.Command}}"},
		{"nested elements", `<a><b>{{.Command}}</b><c/></a>`, "output", ""},
		{"two top-level elements", `<command>{{.Command}}</command><get/>`, "output", "one top-level element"},
		{"whitespace around the element", "\n  <command>{{.Command}}</command>\n", "output", ""},
		{"text after the element", `<command/>{{.Command}}`, "output", "text outside"},
		{"text before the element", `{{.Command}}<command/>`, "output", "text outside"},
		{"command in CDATA", `<command><![CDATA[{{.Command}}]]></command>`, "output", "CDATA"},
		{"command alias in CDATA", `{{$c := .Command}}<command><![CDATA[{{$c}}]]></command>`, "output", "only {{.Command}}"},
		{"transformed command in CDATA", `<command>{{.Command}}<![CDATA[{{html .Command}}]]></command>`, "output", "only {{.Command}}"},
		{"CDATA only in a comment", `<command>{{.Command}}</command><!-- not <![CDATA[{{.Command}}]]> -->`, "output", "comment"},
		{"command only in an attribute", `<command line="{{.Command}}"/>`, "output", "{{.Command}}"},
		{"command only in a comment", `<command><!-- {{.Command}} --></command>`, "output", "{{.Command}}"},
		{"command only in a processing instruction", `<command><?cmd {{.Command}}?></command>`, "output", "{{.Command}}"},
		{"command in an attribute and the text", `<command line="{{.Command}}">{{.Command}}</command>`, "output", ""},
		{"command in the text and a comment", `<command>{{.Command}}<!-- {{.Command}} --></command>`, "output", "comment"},
		{"XML declaration", `<?xml version="1.0"?><command>{{.Command}}</command>`, "output", "XML declaration"},
		{"XML declaration in upper case", `<?XML version="1.0"?><command>{{.Command}}</command>`, "output", "XML declaration"},
		{"other processing instruction", `<?junos-cli x?><command>{{.Command}}</command>`, "output", ""},
		{"command alias in a comment", `{{$c := .Command}}<command>{{$c}}<!-- {{$c}} --></command>`, "output", "only {{.Command}}"},
		{"transformed command in a comment", `<command>{{.Command}}<!-- {{html .Command}} --></command>`, "output", "only {{.Command}}"},
		{"command in a pipeline", `<command>{{.Command | printf "%s"}}</command>`, "output", "only {{.Command}}"},
		{"if action", `<command>{{if .Command}}{{.Command}}{{end}}</command>`, "output", "only {{.Command}}"},
		{"template call", `{{define "c"}}{{.}}{{end}}<command>{{template "c" .Command}}</command>`, "output", "only {{.Command}}"},
		{"trim markers", "<command>\n  {{- .Command -}}\n</command>", "output", ""},
		{"comment around the element", `<!-- note --><command>{{.Command}}</command><!-- end -->`, "output", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := NewNetconfSpec(tc.rpc, tc.output, "")
			if tc.wantErr == "" {
				if err != nil || spec == nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestNewNetconfSpecErrorElement verifies that the error element cannot be the output element.
func TestNewNetconfSpecErrorElement(t *testing.T) {
	_, err := NewNetconfSpec(netconfTestSROSRPC, "md-cli-output-block", "md-cli-output-block")
	if err == nil || !strings.Contains(err.Error(), "netconf.error must name another element") {
		t.Errorf("err = %v, want the netconf.error check", err)
	}
}

// TestNewNetconfSpecElementNames verifies that the output and error elements are XML local names.
func TestNewNetconfSpecElementNames(t *testing.T) {
	cases := []struct {
		name, output, errElem, wantErr string
	}{
		{"names", "md-cli-output-block", "error-message", ""},
		{"prefixed output", "sros:md-cli-output-block", "", "netconf.output must be an XML local name"},
		{"output with a space", "md-cli output", "", "netconf.output must be an XML local name"},
		{"output with markup", "a/><b", "", "netconf.output must be an XML local name"},
		{"output with a trailing colon", "output:", "", "netconf.output must be an XML local name"},
		{"output with a leading colon", ":output", "", "netconf.output must be an XML local name"},
		{"prefixed error", "md-cli-output-block", "sros:error-message", "netconf.error must be an XML local name"},
		{"error with a space", "md-cli-output-block", "error message", "netconf.error must be an XML local name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewNetconfSpec(netconfTestSROSRPC, tc.output, tc.errElem)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestNetconfSpecRender verifies that .Command is XML-escaped.
func TestNetconfSpecRender(t *testing.T) {
	spec := netconfTestSpec(t, netconfTestJunosRPC, "output")
	got, err := spec.Render(`show "a" <b> & 'c'`)
	if err != nil {
		t.Fatal(err)
	}
	want := `<command format="text">show &#34;a&#34; &lt;b&gt; &amp; &#39;c&#39;</command>`
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

// netconfMockRouter is a [Router] that runs its commands over NETCONF.
type netconfMockRouter struct {
	mockRouter
	spec *NetconfSpec
}

func (r netconfMockRouter) Netconf() *NetconfSpec { return r.spec }

// TestRouterInstanceNetconf verifies that RouterInstance picks the NETCONF
// transport from the template and that the health check opens a session.
func TestRouterInstanceNetconf(t *testing.T) {
	srv := startNetconfServer(t, true, false)
	spec := netconfTestSpec(t, netconfTestSROSRPC, "md-cli-output-block")
	ri := &RouterInstance{
		Config:      netconfTestRouter("nc-instance", srv.addr, 0),
		Router:      netconfMockRouter{spec: spec},
		HealthCheck: &HealthCheck{},
	}

	if err := ri.Healthcheck(); err != nil || !ri.HealthCheck.Healthy {
		t.Fatalf("Healthcheck: err=%v healthy=%v", err, ri.HealthCheck.Healthy)
	}
	if _, sessions, commands := srv.stats(); sessions != 1 || len(commands) != 0 {
		t.Errorf("health check: sessions=%d commands=%q, want one session and no commands", sessions, commands)
	}

	out, err := ri.BGPSummary()
	if err != nil {
		t.Fatalf("BGPSummary: %v", err)
	}
	if len(out) != 1 || out[0] != "\nout: bgp.summary\n" {
		t.Errorf("BGPSummary output = %q", out)
	}

	// The SSH dial works, but the device has no NETCONF: unhealthy.
	noNetconf := startNetconfServer(t, true, true)
	riNoNetconf := &RouterInstance{
		Config:      netconfTestRouter("nc-instance-nosubsystem", noNetconf.addr, 0),
		Router:      netconfMockRouter{spec: spec},
		HealthCheck: &HealthCheck{Healthy: true},
	}
	if err := riNoNetconf.Healthcheck(); !errors.Is(err, errs.ExecFailed) || riNoNetconf.HealthCheck.Healthy {
		t.Errorf("Healthcheck without NETCONF: err=%v healthy=%v", err, riNoNetconf.HealthCheck.Healthy)
	}

	// A template that returns no NETCONF settings keeps the SSH exec path.
	riExec := &RouterInstance{
		Config:      netconfTestRouter("nc-instance-exec", noNetconf.addr, 0),
		Router:      netconfMockRouter{},
		HealthCheck: &HealthCheck{},
	}
	if err := riExec.Healthcheck(); err != nil {
		t.Errorf("exec Healthcheck: %v", err)
	}
}
