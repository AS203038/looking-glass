package parse

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	gotextfsm "github.com/sirikothe/gotextfsm"
	"google.golang.org/protobuf/proto"
)

// templates embeds the bundled TextFSM templates. The `all:` prefix
// keeps `.textfsm` and `.tfsm` filenames whose underscores would
// otherwise hide them from the default embed globbing.
//
//go:embed all:textfsm
var templates embed.FS

// resolveTemplate returns the parsed TextFSM body for the named
// template. The lookup order mirrors the YAML-template loader's:
//
//  1. `$ROUTER_DIR/textfsm/<name>.textfsm` (operator override)
//  2. `$ROUTER_DIR/textfsm/<name>.tfsm` (alternate extension)
//  3. embedded `textfsm/<name>.textfsm` (bundled with the binary)
//  4. embedded `textfsm/<name>.tfsm`
//
// Parsed templates are cached per (kind, name) so the gRPC hot path
// pays the parse cost at most once per template. The cache is
// safe under concurrent access via [sync.Map] semantics.
//
// Returns errTemplateMissing when neither source resolved.
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

// errTemplateMissing is the sentinel returned by [resolveTemplate]
// when no template body could be located. Callers convert it to a
// PARSE_STATUS_TEMPLATE_MISSING wire status.
var errTemplateMissing = errors.New("textfsm template not found")

// templateCache memoises parsed TextFSM bodies. Keys are the raw
// template name (without extension); values are gotextfsm.TextFSM.
var templateCache sync.Map

// readTemplateBody locates name's TextFSM source under either the
// operator-supplied ROUTER_DIR overlay or the embedded asset set,
// trying both `.textfsm` and `.tfsm` extensions in order.
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

// TextFSMParser is the [Parser] backed by the gotextfsm engine. A
// single instance per process is enough: it is stateless and the
// per-template parse cache lives in package-level state.
type TextFSMParser struct{}

// Name implements [Parser].
func (TextFSMParser) Name() string { return "textfsm" }

// Parse runs the configured TextFSM template against raw and
// converts the resulting record list into the typed payload for
// op. A missing template surfaces PARSE_STATUS_TEMPLATE_MISSING;
// any other failure surfaces PARSE_STATUS_PARSE_FAILED. Raw bytes
// remain authoritative in both cases.
func (p TextFSMParser) Parse(op Op, raw []byte, cfg Config) Result {
	tpl, err := resolveTemplate(cfg.Template)
	if err != nil {
		if errors.Is(err, errTemplateMissing) {
			log.Printf("PARSE: textfsm template missing op=%s name=%q", op, cfg.Template)
			return Missing(pb.ParserKind_PARSER_KIND_TEXTFSM)
		}
		log.Printf("PARSE: textfsm template %q load failed op=%s: %v", cfg.Template, op, err)
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	out := gotextfsm.ParserOutput{}
	out.Reset(tpl)
	if err := out.ParseTextString(string(raw), tpl, true); err != nil {
		log.Printf("PARSE: textfsm exec failed op=%s name=%q: %v", op, cfg.Template, err)
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	records := out.Dict
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
		log.Printf("PARSE: textfsm parser has no projection for op=%s", op)
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
}

// ---------------------------------------------------------------------------
// Record → typed-message projection
// ---------------------------------------------------------------------------
// Each helper turns the gotextfsm `[]map[string]interface{}` record
// list (already type-coerced according to the .textfsm template's
// `Value` types — `Filldown`, `List`, etc.) into the target
// protobuf message. Field-name lookup is case-insensitive and
// defaults to the zero value on miss, so a template that omits a
// column simply produces a partially-populated struct rather than
// hard-failing.

// recVal fetches the named column from rec, accepting either the
// exact case used in the template or upper-snake-case (the
// TextFSM convention). Returns nil when neither key is present.
func recVal(rec map[string]interface{}, name string) interface{} {
	if v, ok := rec[name]; ok {
		return v
	}
	upper := strings.ToUpper(name)
	if v, ok := rec[upper]; ok {
		return v
	}
	return nil
}

// recString fetches a string column from rec, coercing common
// gotextfsm value shapes (string, []string) into a single string.
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

// recStringList fetches a list-shaped column from rec. The function
// accepts two TextFSM capture shapes interchangeably:
//
//   - a true `Value List` column (already a `[]string` in the
//     gotextfsm record);
//   - a single `Value` capturing the entire whitespace- or
//     comma-separated run in one token (e.g. the trailing path-
//     attribute lines in BGP detail output).
//
// In the latter case the token is split on whitespace and commas
// so callers always see one entry per logical value. This sidesteps
// the gotextfsm gotcha that a repeated `${var}(?:\s+${var})*` line
// only retains the last match when var is declared `Value List`.
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

// splitListToken splits a single captured token on whitespace and
// commas, dropping empty fragments. Used by [recStringList] and
// [recUint32List] when a template captured an entire list as one
// string rather than via `Value List`.
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

// recUint32 best-effort coerces a string column into a uint32.
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

// recUint64 best-effort coerces a string column into a uint64.
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

// recFloat32 best-effort coerces a string column into a float32.
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

// recBool best-effort coerces a string column into a bool.
//
// The accepted truthy tokens cover the spectrum of vendor-specific
// flag conventions: a literal boolean, the FRR/IOS-XE inline `best`
// keyword, the JunOS active-route asterisk `*BGP` marker, the
// Cisco BGP `>` best-path glyph, the MikroTik `A`ctive flag, and
// the Nokia `Best` flag value. Anything else — including the empty
// string — yields false.
func recBool(rec map[string]interface{}, name string) bool {
	switch strings.ToLower(recString(rec, name)) {
	case "true", "1", "yes", "y",
		"*", "*bgp", ">", "a", "best":
		return true
	default:
		return false
	}
}

// recUint32List coerces a list-shaped column of decimal-string AS
// numbers into a []uint32. Used for the BGPPath.as_path column.
//
// Accepts the same two TextFSM capture shapes as [recStringList]:
// a true `Value List` column, or a single token containing the
// entire whitespace- or comma-separated AS-path. Individual
// entries that fail decimal-parse are silently dropped so the
// wire field never carries garbage.
func recUint32List(rec map[string]interface{}, name string) []uint32 {
	raw := recStringList(rec, name)
	// If any of the surviving entries still contain whitespace or
	// commas (e.g. a single `Value as_path (...)` captured the full
	// "65001 65002" or "65001,65002" run in one string), explode
	// them before integer-parsing.
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

// recAgeSeconds coerces a duration-like string (e.g. "1d02h",
// "00:01:30", "90s") into a seconds count. Best-effort; returns 0
// on any unrecognised format so the wire field stays cleanly zero.
func recAgeSeconds(rec map[string]interface{}, name string) uint64 {
	s := strings.TrimSpace(recString(rec, name))
	if s == "" {
		return 0
	}
	// HH:MM:SS form
	if strings.Count(s, ":") == 2 {
		parts := strings.SplitN(s, ":", 3)
		h, _ := strconv.ParseUint(parts[0], 10, 64)
		m, _ := strconv.ParseUint(parts[1], 10, 64)
		sec, _ := strconv.ParseUint(parts[2], 10, 64)
		return h*3600 + m*60 + sec
	}
	// Compound "1d02h03m04s" form
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
	// Bare integer = seconds
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v
	}
	return 0
}

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

// tfsmTracerouteResult projects a flat record list into
// [pb.TracerouteParsed]. Two record shapes are recognised:
//
//   - **Single-probe-per-record** (Arista, FRR via TextFSM, anything
//     using `ip` / `hostname` / `rtt_ms` columns). Records sharing
//     the same TTL are coalesced into one hop with multiple probes,
//     so a vendor whose template emits one line per probe (e.g. a
//     hop line like "1 a.b.c.d 1ms" repeated three times for three
//     probes) still produces a single TracerouteHop with three
//     Probe entries.
//
//   - **Multi-probe-per-record** (Cisco IOS, Juniper, Nokia,
//     MikroTik all emit "1 a.b.c.d 1ms 2ms 3ms" on one line).
//     Templates declare numbered probe columns `ip1`/`rtt_ms1`/
//     `hostname1`, `ip2`/`rtt_ms2`/`hostname2`, … and this projector
//     expands them into N probes per hop. Empty `ipN`/`rtt_msN`
//     pairs (timeouts in slot N) become a synthetic empty probe so
//     the UI can render "1 a.b.c.d 1ms  *  3ms" correctly.
//
// Empty-probe semantics: every hop must have at least one probe so
// the UI can render an explicit "no response" row.
func tfsmTracerouteResult(records []map[string]interface{}) Result {
	if len(records) == 0 {
		return Failed(pb.ParserKind_PARSER_KIND_TEXTFSM)
	}
	tp := &pb.TracerouteParsed{}
	// Header-like fields (`target`, `source`) are typically
	// Filldown'd in the template so every record carries them.
	tp.Target = recString(records[0], "target")
	tp.Source = recString(records[0], "source")
	// Coalesce consecutive records by TTL into a single hop. A TTL
	// of 0 (unset) is treated as "new hop every record" so legacy
	// templates without a TTL column degrade gracefully.
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

// projectProbes turns a single TextFSM record into one or more
// [pb.TracerouteProbe] entries.
//
// The function probes for two column-naming conventions, in order:
//
//  1. Numbered columns `ip1`/`hostname1`/`rtt_ms1`/`asn1`,
//     `ip2`/…, … up to `ip9`. As soon as a slot N is entirely empty
//     (no ip, no rtt, no hostname) iteration stops — slot N is a
//     timeout, but slots N+1, N+2 may have data so we don't break
//     the loop; we emit a synthetic empty probe and continue.
//  2. Plain columns `ip`/`hostname`/`rtt_ms`/`asn` (single-probe
//     templates).
//
// Returns an empty slice when neither convention yields any data.
// The caller is responsible for substituting an empty probe so the
// hop is still visible in the rendered output.
func projectProbes(rec map[string]interface{}) []*pb.TracerouteProbe {
	var probes []*pb.TracerouteProbe
	// Trailing-emptiness is tracked on the raw string slot rather
	// than on the parsed float, so that a valid `0 msec` probe
	// (Cisco's typical first-hop RTT) is not mistaken for an
	// empty slot.
	var rawRTTs []string
	// Numbered convention first.
	sawNumbered := false
	for i := 1; i <= 9; i++ {
		suffix := strconv.Itoa(i)
		// Stop iterating once a slot is entirely undeclared — the
		// template only declared N probes.
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
		// Trim trailing fully-empty probes (no IP, no hostname, no
		// raw RTT capture). A captured `0 msec` keeps the slot.
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

	// Single-probe convention.
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

// columnDeclared reports whether rec contains a key matching name
// (either exact-case or upper-snake-case form). This lets the
// numbered-probe loop distinguish "template declared this column
// but the slot is empty" (continue) from "template never declared
// this column" (stop iterating).
func columnDeclared(rec map[string]interface{}, name string) bool {
	if _, ok := rec[name]; ok {
		return true
	}
	if _, ok := rec[strings.ToUpper(name)]; ok {
		return true
	}
	return false
}


// tfsmBGPSummaryResult projects a BGP-summary record list into the
// typed payload. Several vendor formats overload the trailing
// column of the per-peer row: it carries either a numeric prefix
// count (when the session is Established) or the literal state
// keyword (when it isn't). Templates capturing only `state` for
// the keyword variant and only `prefixes_received` for the numeric
// variant therefore have an under-specified `state` column for
// Established peers; we synthesise the canonical "established"
// keyword here when that pattern is detected.
//
// Conversely, some templates capture the numeric column into the
// `state` slot when it parses as a number (e.g. an Arista
// "PfxRcd 100" cell). When we see a pure-decimal `state` we move
// the value to `prefixes_received` and mark the peer Established.
//
// Common vendor-specific normalisations applied here:
//
//   - Lowercase the state keyword for wire stability.
//   - "estab" / "establ" → "established".
//   - "*" prefix on MikroTik active flag → drop.
//
// Empty-result semantics: a router with zero configured BGP peers
// (or a template preamble that matched the header but found no
// per-peer rows) is a legitimate response, not a parser failure.
// We therefore return PARSE_STATUS_OK with an empty `peers` slice
// rather than PARSE_STATUS_PARSE_FAILED — the latter would force
// the WebUI/CLI back to raw bytes for every quiet router.
func tfsmBGPSummaryResult(records []map[string]interface{}) Result {
	summary := &pb.BGPSummaryParsed{}
	// Filldown'd header context (router_id / local_asn) survives
	// across every record once any record is emitted by gotextfsm.
	// When `records` is empty the template still ran cleanly — we
	// just leave the header fields zero and return an empty peer
	// list.
	if len(records) > 0 {
		summary.LocalAsn = recUint32(records[0], "local_asn")
		summary.RouterId = recString(records[0], "router_id")
	}
	for _, rec := range records {
		// Skip Filldown-only records (no peer_ip).
		ip := recString(rec, "peer_ip")
		if ip == "" {
			continue
		}
		state := strings.ToLower(recString(rec, "state"))
		pfxRecv := recUint64(rec, "prefixes_received")
		pfxAcc := recUint64(rec, "prefixes_accepted")
		pfxSent := recUint64(rec, "prefixes_sent")
		// If the template overloaded the state column with a pure
		// decimal (the Cisco / Arista PfxRcd-on-Established case),
		// migrate it to prefixes_received and mark Established.
		if state != "" && pfxRecv == 0 {
			if v, err := strconv.ParseUint(state, 10, 64); err == nil {
				pfxRecv = v
				state = "established"
			}
		}
		// Conversely, an Established session captured only into
		// prefixes_* with an empty state column also gets the
		// canonical keyword synthesised.
		if state == "" && (pfxRecv > 0 || pfxAcc > 0 || pfxSent > 0) {
			state = "established"
		}
		// Vendor abbreviations → canonical wire keyword.
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

// tfsmBGPPathsResult projects a flat record list into [pb.BGPPaths].
//
// gotextfsm emits a synthesised trailing record carrying only the
// Filldown values when the input ends without a final Record
// transition. We filter those out by skipping records whose
// non-Filldown discriminators (nexthop / peer_ip / as_path) are
// all empty — a real per-path record always populates at least one
// of them.
//
// Empty-result semantics: a BGP lookup that legitimately matched
// zero prefixes (e.g. `show bgp ... community 65000:42` on a
// router that holds no routes tagged with that community) produces
// a template-preamble-only output — header banners with no per-
// path blocks below. gotextfsm runs cleanly against that, emits
// either zero records or only Filldown residue, and we project
// that as PARSE_STATUS_OK with an empty `paths` slice rather than
// PARSE_STATUS_PARSE_FAILED. Returning Failed here would force the
// WebUI/CLI back to raw bytes for every no-hit community / aspath
// / large-community / route query — which is exactly what was
// observed in the field on FRR and Arista EOS before this change.
func tfsmBGPPathsResult(records []map[string]interface{}) Result {
	paths := &pb.BGPPaths{}
	for _, rec := range records {
		// Skip a record that carries no per-path data — usually the
		// trailing Filldown-only residue.
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
