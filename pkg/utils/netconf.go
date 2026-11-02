package utils

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/logging"
	"golang.org/x/crypto/ssh"
	"nemith.io/netconf"
	ncssh "nemith.io/netconf/transport/ssh"
)

// netconfLog is the component-tagged logger for NETCONF events.
var netconfLog = logging.Component("netconf")

// netconfLibLog receives the records of the netconf library at DEBUG. The
// library logs a WARN at the end of every session; this package logs the outcome.
var netconfLibLog = slog.New(debugHandler{netconfLog.Handler()})

// debugHandler passes every record on at DEBUG.
type debugHandler struct{ slog.Handler }

func (h debugHandler) Enabled(ctx context.Context, _ slog.Level) bool {
	return h.Handler.Enabled(ctx, slog.LevelDebug)
}

func (h debugHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Level = slog.LevelDebug
	return h.Handler.Handle(ctx, r)
}

func (h debugHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return debugHandler{h.Handler.WithAttrs(attrs)}
}

func (h debugHandler) WithGroup(name string) slog.Handler {
	return debugHandler{h.Handler.WithGroup(name)}
}

// onceCloseTransport lets the session's receive loop and Session.Close both
// close the transport; the library calls Close from both goroutines.
type onceCloseTransport struct {
	*ncssh.Transport
	once sync.Once
	err  error
}

func (t *onceCloseTransport) Close() error {
	t.once.Do(func() { t.err = t.Transport.Close() })
	return t.err
}

// netconfLogAt emits a structured record at lvl for a NETCONF event.
func netconfLogAt(lvl slog.Level, msg string, router *RouterConfig, extra ...slog.Attr) {
	attrs := routerAttrs(router)
	attrs = append(attrs, extra...)
	netconfLog.LogAttrs(context.Background(), lvl, msg, attrs...)
}

// netconfTimeout bounds each RPC. The library bounds the hello exchange to 30s.
var netconfTimeout = 2 * time.Minute

// netconfCloseTimeout bounds the close-session RPC.
var netconfCloseTimeout = 10 * time.Second

// netconfSampleCommands are rendered at load time to check the RPC template.
// Two distinct commands reject an RPC that hard-codes one of them.
var netconfSampleCommands = [2]string{`show "<&>" 'x'`, `show "<&>" 'y'`}

// NetconfSpec describes how a template runs its commands over NETCONF.
type NetconfSpec struct {
	// RPC renders the body of the <rpc> element for one command.
	RPC *template.Template
	// Output is the local name of the reply elements that hold the command output.
	Output string
	// Error is the local name of the reply elements that hold a command error, or empty.
	Error string
}

// NewNetconfSpec parses the RPC template and checks that it renders
// well-formed XML that contains the command. errElem is optional.
func NewNetconfSpec(rpc, output, errElem string) (*NetconfSpec, error) {
	if rpc == "" {
		return nil, errors.New("netconf.rpc is required")
	}
	if output == "" {
		return nil, errors.New("netconf.output is required")
	}
	if err := checkLocalName("netconf.output", output); err != nil {
		return nil, err
	}
	if errElem != "" {
		if err := checkLocalName("netconf.error", errElem); err != nil {
			return nil, err
		}
	}
	if errElem == output {
		return nil, errors.New("netconf.error must name another element than netconf.output")
	}
	tpl, err := template.New("netconf.rpc").Parse(rpc)
	if err != nil {
		return nil, fmt.Errorf("netconf.rpc: %w", err)
	}
	if err := checkActions(tpl.Tree.Root); err != nil {
		return nil, err
	}
	spec := &NetconfSpec{RPC: tpl, Output: output, Error: errElem}
	for i, cmd := range netconfSampleCommands {
		body, err := spec.Render(cmd)
		if err != nil {
			return nil, fmt.Errorf("netconf.rpc: %w", err)
		}
		if err := checkXML(body); err != nil {
			return nil, fmt.Errorf("netconf.rpc is not a single well-formed XML element: %w", err)
		}
		inText, inCDATA, inComment := commandPlaces(body, cmd)
		if inCDATA {
			return nil, errors.New("netconf.rpc puts {{.Command}} inside CDATA, where its XML escapes reach the device as written")
		}
		if inComment {
			return nil, errors.New("netconf.rpc puts {{.Command}} inside an XML comment, where a -- in the command breaks the XML")
		}
		if !inText || strings.Contains(body, xmlEscape(netconfSampleCommands[1-i])) {
			return nil, errors.New("netconf.rpc does not use {{.Command}} in element text")
		}
	}
	return spec, nil
}

// checkActions reports an error when the template has an action other than
// {{.Command}}: a function, pipeline or variable can change the command where
// commandPlaces does not find it.
func checkActions(root *parse.ListNode) error {
	for _, n := range root.Nodes {
		switch n := n.(type) {
		case *parse.TextNode, *parse.CommentNode:
			continue
		case *parse.ActionNode:
			if len(n.Pipe.Decl) == 0 && len(n.Pipe.Cmds) == 1 && len(n.Pipe.Cmds[0].Args) == 1 {
				if f, ok := n.Pipe.Cmds[0].Args[0].(*parse.FieldNode); ok && len(f.Ident) == 1 && f.Ident[0] == "Command" {
					continue
				}
			}
		}
		return fmt.Errorf("netconf.rpc may use only {{.Command}}, not %s", n)
	}
	return nil
}

// commandPlaces reports whether cmd occurs in the element text of the
// well-formed body, and whether its escaped form occurs inside a CDATA section
// or a comment. The decoder returns each CDATA section as its own CharData
// token, other text unescaped, and comments as written.
func commandPlaces(body, cmd string) (inText, inCDATA, inComment bool) {
	d := xml.NewDecoder(strings.NewReader(body))
	for {
		start := d.InputOffset()
		tok, err := d.RawToken()
		if err != nil {
			return inText, inCDATA, inComment
		}
		switch t := tok.(type) {
		case xml.CharData:
			if strings.HasPrefix(body[start:d.InputOffset()], "<![CDATA[") {
				inCDATA = inCDATA || strings.Contains(string(t), xmlEscape(cmd))
			} else {
				inText = inText || strings.Contains(string(t), cmd)
			}
		case xml.Comment:
			inComment = inComment || strings.Contains(string(t), xmlEscape(cmd))
		}
	}
}

// checkLocalName reports an error when name is not an XML local name without
// a prefix: netconfOutput compares it with the local name of reply elements.
func checkLocalName(field, name string) error {
	var elem struct{ XMLName xml.Name }
	if err := xml.Unmarshal([]byte("<"+name+"/>"), &elem); err != nil || strings.Contains(name, ":") || elem.XMLName.Local != name {
		return fmt.Errorf("%s must be an XML local name without a prefix: %q", field, name)
	}
	return nil
}

// Render returns the RPC body for cmd. The command is XML-escaped before it
// is inserted as {{.Command}}.
func (n *NetconfSpec) Render(cmd string) (string, error) {
	var buf strings.Builder
	if err := n.RPC.Execute(&buf, struct{ Command string }{xmlEscape(cmd)}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func xmlEscape(s string) string {
	var b strings.Builder
	// A strings.Builder never returns a write error.
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// checkXML reports an error when s is not well-formed or does not hold
// exactly one top-level element: an <rpc> carries one operation (RFC 6241 4.1).
func checkXML(s string) error {
	d := xml.NewDecoder(strings.NewReader(s))
	depth, elements, outsideText := 0, 0, false
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				elements++
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(t)) > 0 {
				outsideText = true
			}
		case xml.ProcInst:
			if strings.EqualFold(t.Target, "xml") {
				return errors.New("an XML declaration, which is not allowed inside <rpc>")
			}
		}
	}
	if elements == 0 {
		return errors.New("no XML element")
	}
	if elements > 1 {
		return fmt.Errorf("%d top-level elements, the <rpc> body must hold one top-level element", elements)
	}
	if outsideText {
		return errors.New("text outside the top-level element")
	}
	return nil
}

// netconfOutput collects the text of the reply elements named local, and of
// those named errLocal when it is set.
type netconfOutput struct {
	local    string
	errLocal string
	found    bool
	text     strings.Builder
	errText  strings.Builder
}

// UnmarshalXML walks the <rpc-reply> and keeps the character data inside
// each element named o.local or o.errLocal, at any depth and with any namespace.
func (o *netconfOutput) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	depth, inside, insideErr := 0, 0, 0
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if inside == 0 && t.Name.Local == o.local {
				inside = depth
				o.found = true
			}
			if insideErr == 0 && o.errLocal != "" && t.Name.Local == o.errLocal {
				insideErr = depth
				if o.errText.Len() > 0 {
					o.errText.WriteByte('\n')
				}
			}
		case xml.EndElement:
			if depth == 0 {
				return nil
			}
			if depth == inside {
				inside = 0
			}
			if depth == insideErr {
				insideErr = 0
			}
			depth--
		case xml.CharData:
			if inside > 0 {
				o.text.Write(t)
			}
			if insideErr > 0 {
				o.errText.Write(t)
			}
		}
	}
}

// netconfOnClient opens a NETCONF session on client, runs one RPC per
// command in order and closes the session.
func netconfOnClient(client *ssh.Client, router *RouterConfig, spec *NetconfSpec, cmd []string) ([]string, error) {
	start := time.Now()
	dl := &netconfDeadlines{client: client}
	// The subsystem request and the hello have no context.
	_, setupDone := dl.start(netconfTimeout)
	tr, err := ncssh.NewTransport(client)
	if err != nil {
		setupDone()
		netconfLogAt(slog.LevelError, "netconf subsystem failed", router, slog.Any("err", err))
		return nil, errs.ExecFailed
	}
	sess, err := netconf.NewSession(&onceCloseTransport{Transport: tr}, netconf.WithLogger(netconfLibLog))
	setupDone()
	if err != nil {
		netconfLogAt(slog.LevelError, "netconf hello failed", router, slog.Any("err", err))
		return nil, errs.ExecFailed
	}
	netconfLogAt(slog.LevelDebug, "netconf session open", router,
		slog.Uint64("session_id", sess.SessionID()),
		slog.Duration("duration", time.Since(start)))
	defer func() {
		ctx, done := dl.start(netconfCloseTimeout)
		defer done()
		if dl.expired.Load() {
			// The library skips a send with a canceled context: a write to the closed client races its transport close.
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}
		if err := sess.Close(ctx); err != nil {
			netconfLogAt(slog.LevelDebug, "netconf session close failed", router, slog.Any("err", err))
		}
	}()

	ret := make([]string, len(cmd))
	for i, c := range cmd {
		out, err := netconfCommand(dl, sess, router, spec, i, c)
		if err != nil {
			return nil, err
		}
		ret[i] = out
	}
	return ret, nil
}

// netconfDeadlines closes client when a deadline ends. The library reads the
// rest of a reply without its context, so only a closed client ends a stalled reply.
type netconfDeadlines struct {
	client *ssh.Client
	// expired is set when a deadline closed the client.
	expired atomic.Bool
}

// start returns a context that ends after d. Call done once to stop the
// deadline; when the deadline ended, done waits until the client is closed.
func (n *netconfDeadlines) start(d time.Duration) (ctx context.Context, done func()) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		n.expired.Store(true)
		n.client.Close()
		close(closed)
	})
	return ctx, func() {
		if !stop() {
			<-closed
		}
		cancel()
	}
}

// netconfCommand runs one command and returns the text of its output elements.
func netconfCommand(dl *netconfDeadlines, sess *netconf.Session, router *RouterConfig, spec *NetconfSpec, i int, c string) (string, error) {
	// An RPC can succeed while its deadline closes the client: a write now would race the transport close.
	if dl.expired.Load() {
		netconfLogAt(slog.LevelError, "netconf client closed by an earlier deadline", router,
			slog.Int("cmd_index", i),
			slog.String("cmd", c))
		return "", errs.ExecFailed
	}
	netconfLogAt(slog.LevelDebug, "netconf rpc start", router,
		slog.Int("cmd_index", i),
		slog.String("cmd", c))
	rpcStart := time.Now()
	body, err := spec.Render(c)
	if err != nil {
		netconfLogAt(slog.LevelError, "netconf rpc render failed", router,
			slog.Int("cmd_index", i),
			slog.String("cmd", c),
			slog.Any("err", err))
		return "", errs.ExecFailed
	}
	ctx, done := dl.start(netconfTimeout)
	defer done()
	out := &netconfOutput{local: spec.Output, errLocal: spec.Error}
	err = sess.Exec(ctx, body, out)
	var rpcErrs netconf.RPCErrors
	switch {
	case errors.As(err, &rpcErrs):
		for _, e := range rpcErrs {
			netconfLogAt(slog.LevelError, "netconf rpc-error", router,
				slog.Int("cmd_index", i),
				slog.String("cmd", c),
				slog.String("error_tag", string(e.Tag)),
				slog.String("error_message", truncate(e.Message, 512)))
		}
		return "", errs.ExecFailed
	case err != nil:
		netconfLogAt(slog.LevelError, "netconf rpc failed", router,
			slog.Int("cmd_index", i),
			slog.String("cmd", c),
			slog.Duration("duration", time.Since(rpcStart)),
			slog.Any("err", err))
		return "", errs.ExecFailed
	case !out.found:
		netconfLogAt(slog.LevelError, "netconf reply has no output element", router,
			slog.Int("cmd_index", i),
			slog.String("cmd", c),
			slog.String("output", spec.Output))
		return "", errs.ExecFailed
	}
	netconfLogAt(slog.LevelDebug, "netconf rpc ok", router,
		slog.Int("cmd_index", i),
		slog.Duration("duration", time.Since(rpcStart)),
		slog.Int("output_bytes", out.text.Len()))
	msg := strings.TrimSpace(out.errText.String())
	if msg == "" {
		return out.text.String(), nil
	}
	netconfLogAt(slog.LevelWarn, "netconf command error", router,
		slog.Int("cmd_index", i),
		slog.String("cmd", c),
		slog.String("error_message", truncate(msg, 512)))
	text := out.text.String()
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + msg + "\n", nil
}

// NetconfExec runs each command as one NETCONF RPC in a new session on a new
// SSH connection and returns the per-command output in order. It uses the
// same authentication as [SSHExec]. The pool limits the open connections,
// but no connection is reused: SR OS drops it after close-session. The
// device hostname must point at the NETCONF port (usually 830). Failures
// are returned as the coarse [errs] sentinels.
func NetconfExec(router *RouterConfig, spec *NetconfSpec, cmd []string) ([]string, error) {
	return withSSHClient(router, len(cmd), false, func(client *ssh.Client) ([]string, error) {
		return netconfOnClient(client, router, spec, cmd)
	})
}
