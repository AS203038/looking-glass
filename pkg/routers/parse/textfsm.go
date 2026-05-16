package parse

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	gotextfsm "github.com/sirikothe/gotextfsm"
	"google.golang.org/protobuf/proto"
)

//go:embed all:textfsm
var templates embed.FS

// errTemplateMissing is returned when no template body can be located.
var errTemplateMissing = errors.New("textfsm template not found")

// templateCache memoises parsed TextFSM bodies keyed by template name.
var templateCache sync.Map

// resolveTemplate returns the parsed TextFSM for the named template,
// consulting the operator overlay first and the embedded asset set second.
// Parsed templates are cached for the lifetime of the process.
func resolveTemplate(name string) (gotextfsm.TextFSM, error) {
	if name == "" {
		return gotextfsm.TextFSM{}, errTemplateMissing
	}
	if cached, ok := templateCache.Load(name); ok {
		return cached.(gotextfsm.TextFSM), nil
	}
	body, err := readTemplateBody(name)
	if err != nil {
		return gotextfsm.TextFSM{}, err
	}
	tpl := gotextfsm.TextFSM{}
	if err := tpl.ParseString(body); err != nil {
		return gotextfsm.TextFSM{}, fmt.Errorf("textfsm template %q parse: %w", name, err)
	}
	templateCache.Store(name, tpl)
	return tpl, nil
}

// readTemplateBody locates a template body under `$ROUTER_DIR/textfsm/`
// (operator overlay) or the embedded asset set, trying `.textfsm` then
// `.tfsm`. Returns errTemplateMissing when neither source resolves.
func readTemplateBody(name string) (string, error) {
	rd := os.Getenv("ROUTER_DIR")
	if rd != "" {
		for _, ext := range []string{".textfsm", ".tfsm"} {
			p := filepath.Join(rd, "textfsm", name+ext)
			if data, err := os.ReadFile(p); err == nil {
				return string(data), nil
			}
		}
	}
	for _, ext := range []string{".textfsm", ".tfsm"} {
		p := "textfsm/" + name + ext
		if data, err := templates.ReadFile(p); err == nil {
			return string(data), nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("textfsm template %q: %w", name, err)
		}
	}
	return "", errTemplateMissing
}

// TextFSMParser is the [Parser] backed by the gotextfsm engine.
type TextFSMParser struct{}

// Name implements [Parser].
func (TextFSMParser) Name() string { return "textfsm" }

// Parse runs the configured TextFSM template against raw and projects
// the resulting record list into the typed payload for op.
func (p TextFSMParser) Parse(op Op, raw []byte, cfg Config) Result {
	parseLog.Debug("parser run",
		slog.String("parser", "textfsm"),
		slog.String("op", string(op)),
		slog.String("template", cfg.Template),
		slog.Int("raw_bytes", len(raw)))
	tpl, err := resolveTemplate(cfg.Template)
	if err != nil {
		if errors.Is(err, errTemplateMissing) {
			parseLog.Warn("textfsm template missing",
				slog.String("op", string(op)),
				slog.String("template", cfg.Template))
			return Missing(pb.ParserKind_PARSER_KIND_TEXTFSM)
		}
		parseLog.Error("textfsm template load failed",
			slog.String("op", string(op)),
			slog.String("template", cfg.Template),
			slog.Any("err", err))
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	out := gotextfsm.ParserOutput{}
	out.Reset(tpl)
	if err := out.ParseTextString(string(raw), tpl, true); err != nil {
		parseLog.Error("textfsm exec failed",
			slog.String("op", string(op)),
			slog.String("template", cfg.Template),
			slog.Any("err", err))
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	records := out.Dict
	parseLog.Debug("parser ok",
		slog.String("parser", "textfsm"),
		slog.String("op", string(op)),
		slog.String("template", cfg.Template),
		slog.Int("records", len(records)))
	switch op {
	case OpPing:
		return tfsmPingResult(records)
	case OpTraceroute:
		return tfsmTracerouteResult(records)
	case OpBGPSummary:
		return tfsmBGPSummaryResult(records)
	case OpBGPRoute, OpBGPCommunity, OpBGPLargeCommunity, OpBGPASPath:
		return tfsmBGPPathsResult(records)
	default:
		parseLog.Warn("textfsm parser has no projection",
			slog.String("op", string(op)))
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
}

// recVal returns the value of the named column from rec, accepting the
// template's case or its upper-snake-case form. Returns nil if absent.
func recVal(rec map[string]interface{}, name string) interface{} {
	if v, ok := rec[name]; ok {
		return v
	}
	if v, ok := rec[strings.ToUpper(name)]; ok {
		return v
	}
	return nil
}

// recString returns the named column as a single string. List-shaped
// columns return their first element; absent columns return "".
func recString(rec map[string]interface{}, name string) string {
	switch v := recVal(rec, name).(type) {
	case string:
		return v
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// recStringList returns the named column as a string slice. A `Value List`
// column is returned as-is; a single string is split on whitespace/commas.
func recStringList(rec map[string]interface{}, name string) []string {
	switch v := recVal(rec, name).(type) {
	case []string:
		return v
	case string:
		if v == "" {
			return nil
		}
		return splitListToken(v)
	}
	return nil
}

// splitListToken splits a single token on whitespace and commas,
// dropping empty fragments.
func splitListToken(s string) []string {
	out := make([]string, 0, 4)
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// recUint32 returns the named column parsed as a uint32, or 0 on miss/error.
func recUint32(rec map[string]interface{}, name string) uint32 {
	s := recString(rec, name)
	if s == "" {
		return 0
	}
	if v, err := strconv.ParseUint(s, 10, 32); err == nil {
		return uint32(v)
	}
	return 0
}

// recUint64 returns the named column parsed as a uint64, or 0 on miss/error.
func recUint64(rec map[string]interface{}, name string) uint64 {
	s := recString(rec, name)
	if s == "" {
		return 0
	}
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v
	}
	return 0
}

// recFloat32 returns the named column parsed as a float32, or 0 on miss/error.
func recFloat32(rec map[string]interface{}, name string) float32 {
	s := recString(rec, name)
	if s == "" {
		return 0
	}
	if v, err := strconv.ParseFloat(s, 32); err == nil {
		return float32(v)
	}
	return 0
}

// recBool returns the named column as a bool, recognising the
// vendor-specific best-path markers (`best`, `*`, `>`, `A`, …) plus
// the canonical `true`/`1`/`yes`/`y` tokens.
func recBool(rec map[string]interface{}, name string) bool {
	switch strings.ToLower(recString(rec, name)) {
	case "true", "1", "yes", "y",
		"*", "*bgp", ">", "a", "best":
		return true
	default:
		return false
	}
}

// recUint32List returns the named column as a []uint32, splitting any
// embedded whitespace/comma-separated tokens. Entries that fail to parse
// as decimals are dropped.
func recUint32List(rec map[string]interface{}, name string) []uint32 {
	raw := recStringList(rec, name)
	expanded := make([]string, 0, len(raw))
	for _, s := range raw {
		if strings.ContainsAny(s, " \t,") {
			expanded = append(expanded, splitListToken(s)...)
		} else {
			expanded = append(expanded, s)
		}
	}
	out := make([]uint32, 0, len(expanded))
	for _, s := range expanded {
		if v, err := strconv.ParseUint(s, 10, 32); err == nil {
			out = append(out, uint32(v))
		}
	}
	return out
}

// recAgeSeconds returns the named column parsed as a seconds count.
// Accepts `HH:MM:SS`, compound (`1d02h03m04s`, `1w`) and bare-integer
// forms. Returns 0 on any unrecognised input.
func recAgeSeconds(rec map[string]interface{}, name string) uint64 {
	s := strings.TrimSpace(recString(rec, name))
	if s == "" {
		return 0
	}
	if strings.Count(s, ":") == 2 {
		parts := strings.SplitN(s, ":", 3)
		h, _ := strconv.ParseUint(parts[0], 10, 64)
		m, _ := strconv.ParseUint(parts[1], 10, 64)
		sec, _ := strconv.ParseUint(parts[2], 10, 64)
		return h*3600 + m*60 + sec
	}
	var total uint64
	var cur uint64
	matched := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			cur = cur*10 + uint64(r-'0')
		case r == 'w' || r == 'W':
			total += cur * 7 * 86400
			cur = 0
			matched = true
		case r == 'd' || r == 'D':
			total += cur * 86400
			cur = 0
			matched = true
		case r == 'h' || r == 'H':
			total += cur * 3600
			cur = 0
			matched = true
		case r == 'm' || r == 'M':
			total += cur * 60
			cur = 0
			matched = true
		case r == 's' || r == 'S':
			total += cur
			cur = 0
			matched = true
		}
	}
	if matched {
		return total
	}
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v
	}
	return 0
}

// tfsmPingResult projects a ping record list into [pb.PingStats].
func tfsmPingResult(records []map[string]interface{}) Result {
	if len(records) == 0 {
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	rec := records[0]
	stats := &pb.PingStats{
		Target:          recString(rec, "target"),
		Source:          recString(rec, "source"),
		PacketsSent:     recUint32(rec, "packets_sent"),
		PacketsReceived: recUint32(rec, "packets_received"),
		LossPct:         recFloat32(rec, "loss_pct"),
		RttMinMs:        recFloat32(rec, "rtt_min_ms"),
		RttAvgMs:        recFloat32(rec, "rtt_avg_ms"),
		RttMaxMs:        recFloat32(rec, "rtt_max_ms"),
		RttMdevMs:       recFloat32(rec, "rtt_mdev_ms"),
	}
	if stats.PacketsSent == 0 && stats.PacketsReceived == 0 && stats.Target == "" {
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	return OK(pb.ParserKind_PARSER_KIND_TEXTFSM, proto.Message(stats))
}

// tfsmTracerouteResult projects a traceroute record list into
// [pb.TracerouteParsed], coalescing records sharing the same TTL into a
// single hop with multiple probes. Empty hops produce a single synthetic
// empty probe so the UI can render a "no response" row.
func tfsmTracerouteResult(records []map[string]interface{}) Result {
	if len(records) == 0 {
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	tp := &pb.TracerouteParsed{}
	tp.Target = recString(records[0], "target")
	tp.Source = recString(records[0], "source")
	var current *pb.TracerouteHop
	for _, rec := range records {
		ttl := recUint32(rec, "ttl")
		probes := projectProbes(rec)
		if len(probes) == 0 {
			probes = []*pb.TracerouteProbe{{}}
		}
		if ttl != 0 && current != nil && current.Ttl == ttl {
			current.Probes = append(current.Probes, probes...)
			continue
		}
		current = &pb.TracerouteHop{Ttl: ttl, Probes: probes}
		tp.Hops = append(tp.Hops, current)
	}
	if len(tp.Hops) == 0 {
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	return OK(pb.ParserKind_PARSER_KIND_TEXTFSM, proto.Message(tp))
}

// projectProbes extracts one or more [pb.TracerouteProbe] entries from
// a single record. Numbered columns (`ip1`/`rtt_ms1`/…) take precedence
// over the single-probe `ip`/`rtt_ms`/… convention.
func projectProbes(rec map[string]interface{}) []*pb.TracerouteProbe {
	var probes []*pb.TracerouteProbe
	var rawRTTs []string
	sawNumbered := false
	for i := 1; i <= 9; i++ {
		suffix := strconv.Itoa(i)
		if !columnDeclared(rec, "ip"+suffix) &&
			!columnDeclared(rec, "rtt_ms"+suffix) &&
			!columnDeclared(rec, "hostname"+suffix) {
			break
		}
		sawNumbered = true
		rawRTT := recString(rec, "rtt_ms"+suffix)
		rawIP := recString(rec, "ip"+suffix)
		rawHost := recString(rec, "hostname"+suffix)
		rawRTTs = append(rawRTTs, rawRTT)
		probes = append(probes, &pb.TracerouteProbe{
			Ip:       rawIP,
			Hostname: rawHost,
			RttMs:    recFloat32(rec, "rtt_ms"+suffix),
			Asn:      recUint32(rec, "asn"+suffix),
		})
	}
	if sawNumbered {
		// Trim trailing fully-empty slots so timeouts at the tail of a
		// hop don't materialise as synthetic empty probes.
		for len(probes) > 0 {
			n := len(probes) - 1
			last := probes[n]
			if last.Ip == "" && last.Hostname == "" && rawRTTs[n] == "" && last.Asn == 0 {
				probes = probes[:n]
				rawRTTs = rawRTTs[:n]
				continue
			}
			break
		}
		return probes
	}

	ip := recString(rec, "ip")
	host := recString(rec, "hostname")
	rtt := recFloat32(rec, "rtt_ms")
	asn := recUint32(rec, "asn")
	if ip == "" && host == "" && rtt == 0 && asn == 0 {
		return nil
	}
	return []*pb.TracerouteProbe{{
		Ip: ip, Hostname: host, RttMs: rtt, Asn: asn,
	}}
}

// columnDeclared reports whether rec contains a key matching name, in
// either the exact-case or upper-snake-case form.
func columnDeclared(rec map[string]interface{}, name string) bool {
	if _, ok := rec[name]; ok {
		return true
	}
	if _, ok := rec[strings.ToUpper(name)]; ok {
		return true
	}
	return false
}

// tfsmBGPSummaryResult projects a BGP-summary record list into
// [pb.BGPSummaryParsed]. The state column is normalised across vendor
// conventions (numeric prefix counts on Established sessions are migrated
// to prefixes_received, and vendor abbreviations are expanded). An empty
// peer list is reported as OK, not Failed.
func tfsmBGPSummaryResult(records []map[string]interface{}) Result {
	summary := &pb.BGPSummaryParsed{}
	if len(records) > 0 {
		summary.LocalAsn = recUint32(records[0], "local_asn")
		summary.RouterId = recString(records[0], "router_id")
	}
	for _, rec := range records {
		ip := recString(rec, "peer_ip")
		if ip == "" {
			continue
		}
		state := strings.ToLower(recString(rec, "state"))
		pfxRecv := recUint64(rec, "prefixes_received")
		pfxAcc := recUint64(rec, "prefixes_accepted")
		pfxSent := recUint64(rec, "prefixes_sent")
		if state != "" && pfxRecv == 0 {
			if v, err := strconv.ParseUint(state, 10, 64); err == nil {
				pfxRecv = v
				state = "established"
			}
		}
		if state == "" && (pfxRecv > 0 || pfxAcc > 0 || pfxSent > 0) {
			state = "established"
		}
		switch state {
		case "estab", "establ", "e":
			state = "established"
		}
		peer := &pb.BGPPeer{
			PeerIp:           ip,
			PeerAsn:          recUint32(rec, "peer_asn"),
			Description:      recString(rec, "description"),
			State:            state,
			StateDetail:      recString(rec, "state_detail"),
			UptimeSeconds:    recAgeSeconds(rec, "uptime"),
			PrefixesReceived: pfxRecv,
			PrefixesAccepted: pfxAcc,
			PrefixesSent:     pfxSent,
			AddressFamily:    recString(rec, "address_family"),
		}
		summary.Peers = append(summary.Peers, peer)
	}
	return OK(pb.ParserKind_PARSER_KIND_TEXTFSM, proto.Message(summary))
}

// tfsmBGPPathsResult projects a BGP-paths record list into [pb.BGPPaths].
// Records carrying no per-path discriminators (nexthop / peer_ip / as_path)
// are skipped to filter trailing Filldown-only residue. Empty path lists
// are reported as OK, not Failed.
func tfsmBGPPathsResult(records []map[string]interface{}) Result {
	paths := &pb.BGPPaths{}
	for _, rec := range records {
		if recString(rec, "nexthop") == "" &&
			recString(rec, "peer_ip") == "" &&
			recString(rec, "as_path") == "" &&
			len(recStringList(rec, "as_path")) == 0 {
			continue
		}
		path := &pb.BGPPath{
			Prefix:           recString(rec, "prefix"),
			Nexthop:          recString(rec, "nexthop"),
			AsPath:           recUint32List(rec, "as_path"),
			Origin:           strings.ToLower(recString(rec, "origin")),
			Med:              recUint32(rec, "med"),
			LocalPref:        recUint32(rec, "local_pref"),
			Communities:      recStringList(rec, "communities"),
			LargeCommunities: recStringList(rec, "large_communities"),
			Best:             recBool(rec, "best"),
			PeerAsn:          recUint32(rec, "peer_asn"),
			PeerIp:           recString(rec, "peer_ip"),
			AgeSeconds:       recAgeSeconds(rec, "age"),
		}
		paths.Paths = append(paths.Paths, path)
	}
	return OK(pb.ParserKind_PARSER_KIND_TEXTFSM, proto.Message(paths))
}
