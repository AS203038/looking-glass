package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

// ANSI colour escapes (gated by isColorEnabled).
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiDim    = "\x1b[2m"
	ansiBold   = "\x1b[1m"
)

// isTTY returns true if `f` is an interactive terminal. We probe via Stat()
// and check the character-device mode bit to avoid pulling in a dep.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// isColorEnabled returns true if pretty output should emit ANSI colour. We
// honour --no-color (and the NO_COLOR / LG_NO_COLOR env vars baked into the
// default), and additionally turn colour off when stdout is not a TTY (e.g.
// piped or redirected).
func isColorEnabled() bool {
	if opts.NoColor {
		return false
	}
	return isTTY(os.Stdout)
}

// colorize wraps `s` in `code` + reset if colour is enabled.
func colorize(s, code string) string {
	if !isColorEnabled() {
		return s
	}
	return code + s + ansiReset
}

// opResult is the canonical shape we emit for ping/traceroute/bgp queries.
type opResult struct {
	Result    string `json:"result"`
	Timestamp string `json:"timestamp"`
	// Parsed is the structured payload, included when populated by
	// the server. Type is `any` because each operation returns a
	// different protobuf message; we let `encoding/json` handle the
	// reflection-based serialisation.
	Parsed any `json:"parsed,omitempty"`
	// ParserKind is the symbolic provenance label, e.g. "textfsm".
	ParserKind string `json:"parser_kind,omitempty"`
	// ParseStatus is the symbolic parse outcome, e.g. "ok".
	ParseStatus string `json:"parse_status,omitempty"`
}

// parserKindLabel maps a [pb.ParserKind] to its short string form
// for `--output json` and the pretty footer.
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

// printOpResult writes the result of a single-router operation according to
// the global --output mode.
//
// Modes:
//   - raw    : just the result body, no timestamp, no trailing newline added
//   - json   : {"result": "...", "timestamp": "..."}\n
//   - pretty : result body, then (unless --quiet) a dim "ts: <RFC3339>" footer
//     on stderr so stdout stays clean for piping.
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
	default: // pretty
		if _, err := io.WriteString(os.Stdout, result); err != nil {
			return err
		}
		// Ensure result ends with a newline before the footer.
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
//
// When `--output pretty` and parsed is non-nil and parseStatus is OK,
// emit a typed pretty-printer (one of [printPingPretty],
// [printTraceroutePretty], …) instead of dumping the raw bytes. The
// raw bytes are still available — under `--output raw` they are
// always returned verbatim — but `pretty` mode is the only place
// where the structured form is the primary view.
//
// When `--output json`, the parsed payload is serialised alongside
// the raw bytes so downstream scripts can `jq .parsed` directly.
//
// parsed may be nil; the function then degrades to [printOpResult]'s
// behaviour exactly.
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
	default: // pretty
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
			// Unknown payload — fall back to raw.
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
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if s.GetTarget() != "" {
		fmt.Fprintf(w, "target:\t%s\n", s.GetTarget())
	}
	if s.GetSource() != "" {
		fmt.Fprintf(w, "source:\t%s\n", s.GetSource())
	}
	lossColor := ansiGreen
	if s.GetLossPct() >= 50 {
		lossColor = ansiRed
	} else if s.GetLossPct() > 0 {
		lossColor = ansiYellow
	}
	fmt.Fprintf(w, "packets:\t%d sent, %d received, %s loss\n",
		s.GetPacketsSent(), s.GetPacketsReceived(),
		colorize(fmt.Sprintf("%.0f%%", s.GetLossPct()), lossColor))
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

// printTraceroutePretty renders a [pb.TracerouteParsed] as a hop
// table.
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
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, colorize("TTL\tHOST\tIP\tRTT", ansiBold))
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

// printBGPSummaryPretty renders a [pb.BGPSummaryParsed] as a peer
// table with state-coloured badges.
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
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, colorize("PEER\tASN\tAF\tSTATE\tUPTIME\tPFX_IN\tPFX_OUT", ansiBold))
	for _, p := range s.GetPeers() {
		state := p.GetState()
		var sColor string
		switch state {
		case "established":
			sColor = ansiGreen
		case "idle", "connect":
			sColor = ansiYellow
		default:
			sColor = ansiRed
		}
		af := strings.TrimSuffix(p.GetAddressFamily(), "-unicast")
		if af == "" {
			af = "—"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%d\t%d\n",
			p.GetPeerIp(), p.GetPeerAsn(), af,
			colorize(state, sColor),
			fmtDuration(p.GetUptimeSeconds()),
			p.GetPrefixesReceived(), p.GetPrefixesSent())
	}
	w.Flush()
	fmt.Fprintf(os.Stdout, "%d peer(s)\n", len(s.GetPeers()))
}

// printBGPPathsPretty renders a [pb.BGPPaths] as a paths table.
// Columns: BEST glyph, PREFIX, NEXTHOP, MED, LOCAL_PREF, AS_PATH,
// PEER, COMMUNITIES. MED and LOCAL_PREF render the canonical
// "unset" character (—) when zero rather than the literal "0" so
// the table doesn't lie about attributes a vendor may have omitted.
func printBGPPathsPretty(paths *pb.BGPPaths) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, colorize("BEST\tPREFIX\tNEXTHOP\tMED\tLOCPREF\tAS_PATH\tPEER\tCOMMUNITIES", ansiBold))
	for _, p := range paths.GetPaths() {
		best := " "
		if p.GetBest() {
			best = colorize(">", ansiGreen)
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


// fmtDuration renders a seconds count as a compact "1d02h" /
// "2h05m" / "30m" / "45s" string.
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

// printJSON writes `v` as a JSON object to stdout (used by `instances`,
// `routers`, `info` in --output json mode).
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
