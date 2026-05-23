package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

// ANSI colour escape sequences emitted to the terminal.
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiDim    = "\x1b[2m"
	ansiBold   = "\x1b[1m"
)

// HTML-tag sentinels recognised by [colorTabWriter]. They are zero-width
// to [text/tabwriter] under [tabwriter.FilterHTML] and are substituted
// with their ANSI equivalents at flush time.
const (
	cReset  = "<c:reset>"
	cGreen  = "<c:green>"
	cRed    = "<c:red>"
	cYellow = "<c:yellow>"
	cDim    = "<c:dim>"
	cBold   = "<c:bold>"
)

// isTTY reports whether f is an interactive terminal.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// isColorEnabled reports whether pretty output should emit ANSI colour.
func isColorEnabled() bool {
	if opts.ForceColor {
		return true
	}
	if opts.NoColor {
		return false
	}
	return isTTY(os.Stdout)
}

// colorize wraps s with ANSI escape code and reset when colour is enabled.
// It is intended for output that bypasses [colorTabWriter].
func colorize(s, code string) string {
	if !isColorEnabled() {
		return s
	}
	return code + s + ansiReset
}

// colorTabWriter is a [text/tabwriter] that accepts the c* HTML-tag
// colour sentinels in its input. Column widths are computed treating the
// sentinels as zero width; ANSI escapes are substituted only after Flush
// so they never inflate column padding.
type colorTabWriter struct {
	buf *bytes.Buffer
	tw  *tabwriter.Writer
	out io.Writer
}

// newTabWriter returns a [colorTabWriter] writing to out.
func newTabWriter(out io.Writer) *colorTabWriter {
	buf := new(bytes.Buffer)
	return &colorTabWriter{
		buf: buf,
		tw:  tabwriter.NewWriter(buf, 0, 0, 2, ' ', tabwriter.FilterHTML),
		out: out,
	}
}

// Write forwards p to the underlying [tabwriter.Writer].
func (w *colorTabWriter) Write(p []byte) (int, error) {
	return w.tw.Write(p)
}

// Flush finalises the table, substitutes colour sentinels for their ANSI
// equivalents (or strips them when colour is disabled), and writes the
// result to the configured output.
func (w *colorTabWriter) Flush() error {
	if err := w.tw.Flush(); err != nil {
		return err
	}
	b := w.buf.Bytes()
	if isColorEnabled() {
		b = bytes.ReplaceAll(b, []byte(cReset), []byte(ansiReset))
		b = bytes.ReplaceAll(b, []byte(cGreen), []byte(ansiGreen))
		b = bytes.ReplaceAll(b, []byte(cRed), []byte(ansiRed))
		b = bytes.ReplaceAll(b, []byte(cYellow), []byte(ansiYellow))
		b = bytes.ReplaceAll(b, []byte(cDim), []byte(ansiDim))
		b = bytes.ReplaceAll(b, []byte(cBold), []byte(ansiBold))
	} else {
		b = bytes.ReplaceAll(b, []byte(cReset), nil)
		b = bytes.ReplaceAll(b, []byte(cGreen), nil)
		b = bytes.ReplaceAll(b, []byte(cRed), nil)
		b = bytes.ReplaceAll(b, []byte(cYellow), nil)
		b = bytes.ReplaceAll(b, []byte(cDim), nil)
		b = bytes.ReplaceAll(b, []byte(cBold), nil)
	}
	_, err := w.out.Write(b)
	w.buf.Reset()
	return err
}

// opResult is the canonical JSON shape emitted for ping/traceroute/bgp queries.
type opResult struct {
	Result    string `json:"result"`
	Timestamp string `json:"timestamp"`
	// Parsed is the structured payload, when populated.
	Parsed any `json:"parsed,omitempty"`
	// ParserKind is the symbolic provenance label.
	ParserKind string `json:"parser_kind,omitempty"`
	// ParseStatus is the symbolic parse outcome.
	ParseStatus string `json:"parse_status,omitempty"`
}

// parserKindLabel maps a [pb.ParserKind] to its short string form.
func parserKindLabel(k pb.ParserKind) string {
	switch k {
	case pb.ParserKind_PARSER_KIND_TEXTFSM:
		return "textfsm"
	case pb.ParserKind_PARSER_KIND_NATIVE_JSON:
		return "native_json"
	case pb.ParserKind_PARSER_KIND_BUILTIN:
		return "builtin"
	default:
		return ""
	}
}

// parseStatusLabel maps a [pb.ParseStatus] to its short string form.
func parseStatusLabel(s pb.ParseStatus) string {
	switch s {
	case pb.ParseStatus_PARSE_STATUS_OK:
		return "ok"
	case pb.ParseStatus_PARSE_STATUS_DISABLED:
		return "disabled"
	case pb.ParseStatus_PARSE_STATUS_TEMPLATE_MISSING:
		return "template_missing"
	case pb.ParseStatus_PARSE_STATUS_PARSE_FAILED:
		return "parse_failed"
	default:
		return ""
	}
}

// printOpResult writes the result of a single-router operation
// according to the global --output mode.
func printOpResult(result string, ts time.Time) error {
	switch opts.Output {
	case "raw":
		_, err := io.WriteString(os.Stdout, result)
		return err
	case "json":
		return json.NewEncoder(os.Stdout).Encode(opResult{
			Result:    result,
			Timestamp: ts.Format(time.RFC3339),
		})
	default:
		if _, err := io.WriteString(os.Stdout, result); err != nil {
			return err
		}
		if len(result) == 0 || result[len(result)-1] != '\n' {
			fmt.Fprintln(os.Stdout)
		}
		if !opts.Quiet {
			fmt.Fprintln(os.Stderr, colorize(
				fmt.Sprintf("ts: %s", ts.Format(time.RFC3339)), ansiDim,
			))
		}
		return nil
	}
}

// printParsedResult is the structured-aware variant of [printOpResult].
// When parsed is nil it degrades to [printOpResult]'s behaviour.
func printParsedResult(result string, ts time.Time, parsed any, kind pb.ParserKind, status pb.ParseStatus) error {
	switch opts.Output {
	case "raw":
		_, err := io.WriteString(os.Stdout, result)
		return err
	case "json":
		return json.NewEncoder(os.Stdout).Encode(opResult{
			Result:      result,
			Timestamp:   ts.Format(time.RFC3339),
			Parsed:      parsed,
			ParserKind:  parserKindLabel(kind),
			ParseStatus: parseStatusLabel(status),
		})
	default:
		if parsed == nil || status != pb.ParseStatus_PARSE_STATUS_OK {
			return printOpResult(result, ts)
		}
		switch p := parsed.(type) {
		case *pb.PingStats:
			printPingPretty(p)
		case *pb.TracerouteParsed:
			printTraceroutePretty(p)
		case *pb.BGPSummaryParsed:
			printBGPSummaryPretty(p)
		case *pb.BGPPaths:
			printBGPPathsPretty(p)
		default:
			return printOpResult(result, ts)
		}
		if !opts.Quiet {
			label := parserKindLabel(kind)
			if label == "" {
				label = "unknown"
			}
			fmt.Fprintln(os.Stderr, colorize(
				fmt.Sprintf("ts: %s · parser: %s", ts.Format(time.RFC3339), label), ansiDim,
			))
		}
		return nil
	}
}

// printPingPretty renders a [pb.PingStats] as a compact stat block.
func printPingPretty(s *pb.PingStats) {
	w := newTabWriter(os.Stdout)
	if s.GetTarget() != "" {
		fmt.Fprintf(w, "target:\t%s\n", s.GetTarget())
	}
	if s.GetSource() != "" {
		fmt.Fprintf(w, "source:\t%s\n", s.GetSource())
	}
	lossColor := cGreen
	if s.GetLossPct() >= 50 {
		lossColor = cRed
	} else if s.GetLossPct() > 0 {
		lossColor = cYellow
	}
	lossStr := fmt.Sprintf("%s%.0f%%%s", lossColor, s.GetLossPct(), cReset)
	fmt.Fprintf(w, "packets:\t%d sent, %d received, %s loss\n",
		s.GetPacketsSent(), s.GetPacketsReceived(), lossStr)
	if s.GetRttAvgMs() > 0 || s.GetRttMaxMs() > 0 {
		fmt.Fprintf(w, "rtt min/avg/max:\t%.2f / %.2f / %.2f ms",
			s.GetRttMinMs(), s.GetRttAvgMs(), s.GetRttMaxMs())
		if s.GetRttMdevMs() > 0 {
			fmt.Fprintf(w, "  (mdev %.2f)", s.GetRttMdevMs())
		}
		fmt.Fprintln(w)
	}
	w.Flush()
}

// printTraceroutePretty renders a [pb.TracerouteParsed] as a hop table.
func printTraceroutePretty(tp *pb.TracerouteParsed) {
	if tp.GetTarget() != "" || tp.GetSource() != "" {
		if tp.GetTarget() != "" {
			fmt.Fprintf(os.Stdout, "target: %s", tp.GetTarget())
		}
		if tp.GetSource() != "" {
			fmt.Fprintf(os.Stdout, "  source: %s", tp.GetSource())
		}
		fmt.Fprintln(os.Stdout)
	}
	w := newTabWriter(os.Stdout)
	fmt.Fprintln(w, cBold+"TTL\tHOST\tIP\tRTT"+cReset)
	for _, hop := range tp.GetHops() {
		probes := hop.GetProbes()
		host, ip, rtts := "—", "*", []string{}
		if len(probes) > 0 {
			if probes[0].GetHostname() != "" {
				host = probes[0].GetHostname()
			}
			if probes[0].GetIp() != "" {
				ip = probes[0].GetIp()
			}
			for _, p := range probes {
				if p.GetRttMs() > 0 {
					rtts = append(rtts, fmt.Sprintf("%.1f ms", p.GetRttMs()))
				} else {
					rtts = append(rtts, "*")
				}
			}
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
			hop.GetTtl(), host, ip, strings.Join(rtts, "  "))
	}
	w.Flush()
}

// printBGPSummaryPretty renders a [pb.BGPSummaryParsed] as a peer table.
func printBGPSummaryPretty(s *pb.BGPSummaryParsed) {
	if s.GetLocalAsn() != 0 || s.GetRouterId() != "" {
		if s.GetLocalAsn() != 0 {
			fmt.Fprintf(os.Stdout, "local AS: %d", s.GetLocalAsn())
		}
		if s.GetRouterId() != "" {
			fmt.Fprintf(os.Stdout, "  router-id: %s", s.GetRouterId())
		}
		fmt.Fprintln(os.Stdout)
	}
	w := newTabWriter(os.Stdout)
	fmt.Fprintln(w, cBold+"PEER\tASN\tAF\tSTATE\tUPTIME\tPFX_IN\tPFX_OUT"+cReset)
	for _, p := range s.GetPeers() {
		state := p.GetState()
		var sColor string
		switch state {
		case "established":
			sColor = cGreen
		case "idle", "connect":
			sColor = cYellow
		default:
			sColor = cRed
		}
		af := strings.TrimSuffix(p.GetAddressFamily(), "-unicast")
		if af == "" {
			af = "—"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s%s%s\t%s\t%d\t%d\n",
			p.GetPeerIp(), p.GetPeerAsn(), af,
			sColor, state, cReset,
			fmtDuration(p.GetUptimeSeconds()),
			p.GetPrefixesReceived(), p.GetPrefixesSent())
	}
	w.Flush()
	fmt.Fprintf(os.Stdout, "%d peer(s)\n", len(s.GetPeers()))
}

// printBGPPathsPretty renders a [pb.BGPPaths] as a paths table.
func printBGPPathsPretty(paths *pb.BGPPaths) {
	w := newTabWriter(os.Stdout)
	fmt.Fprintln(w, cBold+"BEST\tPREFIX\tNEXTHOP\tMED\tLOCPREF\tAS_PATH\tPEER\tCOMMUNITIES"+cReset)
	for _, p := range paths.GetPaths() {
		best := " "
		if p.GetBest() {
			best = cGreen + ">" + cReset
		}
		asPath := ""
		for i, a := range p.GetAsPath() {
			if i > 0 {
				asPath += " "
			}
			asPath += fmt.Sprintf("%d", a)
		}
		if asPath == "" {
			asPath = "—"
		}
		med := "—"
		if p.GetMed() != 0 {
			med = fmt.Sprintf("%d", p.GetMed())
		}
		locPref := "—"
		if p.GetLocalPref() != 0 {
			locPref = fmt.Sprintf("%d", p.GetLocalPref())
		}
		peer := "—"
		if p.GetPeerAsn() != 0 || p.GetPeerIp() != "" {
			peer = fmt.Sprintf("AS%d %s", p.GetPeerAsn(), p.GetPeerIp())
		}
		comms := append([]string{}, p.GetCommunities()...)
		comms = append(comms, p.GetLargeCommunities()...)
		commsStr := strings.Join(comms, " ")
		if commsStr == "" {
			commsStr = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			best, p.GetPrefix(), p.GetNexthop(), med, locPref, asPath, peer, commsStr)
	}
	w.Flush()
	fmt.Fprintf(os.Stdout, "%d path(s)\n", len(paths.GetPaths()))
}

// fmtDuration renders a seconds count as a compact "1d02h"/"2h05m"/"30m"/"45s" string.
func fmtDuration(sec uint64) string {
	if sec == 0 {
		return "—"
	}
	d := sec / 86400
	h := (sec % 86400) / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd%02dh", d, h)
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// printJSON writes v as indented JSON to stdout.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
