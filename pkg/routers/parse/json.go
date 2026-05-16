package parse

import (
	"encoding/json"
	"log/slog"
	"strings"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"google.golang.org/protobuf/proto"
)

// JSONParser is the [Parser] backed by vendor-native JSON output.
// The schema selector (Config.Schema) chooses the input shape:
// "frr_bgp_route_v1" for FRR route/community/aspath lookups, and
// "frr_bgp_summary_v1" for FRR summary lookups.
type JSONParser struct{}

// Name implements [Parser].
func (JSONParser) Name() string { return "native_json" }

// Parse implements [Parser] by dispatching on op.
func (p JSONParser) Parse(op Op, raw []byte, cfg Config) Result {
	parseLog.Debug("parser run",
		slog.String("parser", "native_json"),
		slog.String("op", string(op)),
		slog.String("schema", cfg.Schema),
		slog.Int("raw_bytes", len(raw)))
	switch op {
	case OpBGPRoute, OpBGPCommunity, OpBGPLargeCommunity, OpBGPASPath:
		res := parseFRRBGPPaths(raw)
		if bp, ok := res.Payload.(*pb.BGPPaths); ok {
			parseLog.Debug("parser ok",
				slog.String("parser", "native_json"),
				slog.String("op", string(op)),
				slog.String("schema", cfg.Schema),
				slog.Int("paths", len(bp.Paths)))
		}
		return res
	case OpBGPSummary:
		res := parseFRRBGPSummary(raw)
		if bs, ok := res.Payload.(*pb.BGPSummaryParsed); ok {
			parseLog.Debug("parser ok",
				slog.String("parser", "native_json"),
				slog.String("op", string(op)),
				slog.String("schema", cfg.Schema),
				slog.Int("peers", len(bs.Peers)))
		}
		return res
	default:
		parseLog.Warn("native_json parser has no handler",
			slog.String("op", string(op)),
			slog.String("schema", cfg.Schema))
		return Missing(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
}

// frrBGPRoutesEnvelope is the multi-prefix "routes" map shape used
// by FRR's community / large-community / aspath / full-table queries.
type frrBGPRoutesEnvelope struct {
	VrfName  string                  `json:"vrfName"`
	RouterID string                  `json:"routerId"`
	Routes   map[string][]frrBGPPath `json:"routes"`
}

// frrBGPPath is one path entry in an FRR BGP JSON response.
type frrBGPPath struct {
	Valid bool `json:"valid"`
	// Bestpath accepts both the bare-bool and the {"overall": bool}
	// forms FRR emits across versions; extracted via frrBestpath.
	Bestpath json.RawMessage `json:"bestpath"`
	Med      uint32          `json:"med"`
	Metric   uint32          `json:"metric"`
	LocPrf   uint32          `json:"locPrf"`
	// ASPath holds FRR's AS-path under the "aspath" key; Path holds
	// the v9.x+ alternative "path" key. Both are extracted via
	// extractFRRASPathString to handle string and object forms.
	ASPath   json.RawMessage  `json:"aspath"`
	Path     json.RawMessage  `json:"path"`
	Origin   string           `json:"origin"`
	Comm     *frrBGPStringObj `json:"community,omitempty"`
	LargeC   *frrBGPStringObj `json:"largeCommunity,omitempty"`
	Nexthops []frrBGPNexthop  `json:"nexthops"`
	Peer     *frrBGPPeerRef   `json:"peer,omitempty"`
	// PeerID is the path-level peer id used by single-prefix detail
	// when the nested Peer object is absent.
	PeerID     string            `json:"peerId"`
	Uptime     string            `json:"uptime"`
	LastUpdate *frrBGPLastUpdate `json:"lastUpdate,omitempty"`
}

// frrBestpath extracts the boolean best-path flag from FRR's
// dual-shape bestpath field. Accepts a bare bool, the object
// {"overall": bool}, or a missing field (returns false).
func frrBestpath(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	var obj struct {
		Overall bool `json:"overall"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Overall
	}
	return false
}

// frrBGPStringObj is the {"string": "..."} wrapper FRR uses for
// community / large-community / aspath values.
type frrBGPStringObj struct {
	String string `json:"string"`
}

// frrBGPASPathObject is FRR's v9.x+ aspath object form.
type frrBGPASPathObject struct {
	String string `json:"string"`
	Length uint32 `json:"length"`
}

// frrBGPNexthop is one nexthop entry on an [frrBGPPath].
type frrBGPNexthop struct {
	IP  string `json:"ip"`
	AFI string `json:"afi"`
}

// frrBGPPeerRef is the nested peer reference on an [frrBGPPath].
type frrBGPPeerRef struct {
	PeerID string `json:"peerId"`
	ASN    uint32 `json:"asn"`
}

// frrBGPLastUpdate is the v9.x+ replacement for the path-level
// Uptime field; String carries the compact duration text.
type frrBGPLastUpdate struct {
	Epoch  json.Number `json:"epoch"`
	String string      `json:"string"`
}

// parseFRRBGPPaths decodes one or more FRR BGP route-list JSON
// envelopes (concatenated when a template ran v4 and v6 commands
// in sequence) into [pb.BGPPaths]. An envelope that decoded
// structurally but produced zero paths is reported as OK with an
// empty payload; PARSE_FAILED is reserved for the case where no
// envelope shape matched anywhere.
func parseFRRBGPPaths(raw []byte) Result {
	paths := &pb.BGPPaths{}
	chunks := splitJSONObjects(raw)
	if len(chunks) == 0 {
		parseLog.Warn("frr bgp json: no top-level JSON objects found")
		return Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
	decodedAny := false
	for _, chunk := range chunks {
		var env frrBGPRoutesEnvelope
		if err := json.Unmarshal(chunk, &env); err == nil && env.Routes != nil {
			decodedAny = true
			for prefix, ps := range env.Routes {
				for _, p := range ps {
					paths.Paths = append(paths.Paths, projectFRRPath(prefix, p))
				}
			}
			continue
		}
		if tryFRRSinglePrefixJSON(chunk, paths) {
			decodedAny = true
			continue
		}
		parseLog.Warn("frr bgp json: neither routes-map nor single-prefix envelope matched")
	}
	if !decodedAny {
		return Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
	return OK(pb.ParserKind_PARSER_KIND_NATIVE_JSON, proto.Message(paths))
}

// tryFRRSinglePrefixJSON decodes the {prefix, paths: []} envelope
// shape and appends every path onto paths. Returns true on success.
func tryFRRSinglePrefixJSON(chunk []byte, paths *pb.BGPPaths) bool {
	var entry struct {
		Prefix  string       `json:"prefix"`
		Paths   []frrBGPPath `json:"paths"`
		Network string       `json:"network"`
	}
	if err := json.Unmarshal(chunk, &entry); err != nil {
		return false
	}
	prefix := entry.Prefix
	if prefix == "" {
		prefix = entry.Network
	}
	if prefix == "" || len(entry.Paths) == 0 {
		return false
	}
	for _, p := range entry.Paths {
		paths.Paths = append(paths.Paths, projectFRRPath(prefix, p))
	}
	return true
}

// extractFRRASPathString returns the AS-path string from raw,
// accepting both the bare-string and {"string": "..."} forms.
func extractFRRASPathString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj frrBGPASPathObject
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.String
	}
	return ""
}

// projectFRRPath maps an [frrBGPPath] onto the wire [pb.BGPPath].
func projectFRRPath(prefix string, p frrBGPPath) *pb.BGPPath {
	out := &pb.BGPPath{
		Prefix:    prefix,
		Origin:    strings.ToLower(p.Origin),
		LocalPref: p.LocPrf,
		Best:      frrBestpath(p.Bestpath),
	}
	if p.Med != 0 {
		out.Med = p.Med
	} else {
		out.Med = p.Metric
	}
	if len(p.Nexthops) > 0 {
		out.Nexthop = p.Nexthops[0].IP
	}
	if p.Peer != nil {
		out.PeerIp = p.Peer.PeerID
		out.PeerAsn = p.Peer.ASN
	} else if p.PeerID != "" {
		out.PeerIp = p.PeerID
	}
	asPathStr := extractFRRASPathString(p.ASPath)
	if asPathStr == "" {
		asPathStr = extractFRRASPathString(p.Path)
	}
	if asPathStr != "" {
		for _, tok := range strings.Fields(asPathStr) {
			var asn uint32
			for _, r := range tok {
				if r < '0' || r > '9' {
					asn = 0
					break
				}
				asn = asn*10 + uint32(r-'0')
			}
			if asn != 0 {
				out.AsPath = append(out.AsPath, asn)
			}
		}
	}
	if p.Comm != nil && p.Comm.String != "" {
		out.Communities = strings.Fields(p.Comm.String)
	}
	if p.LargeC != nil && p.LargeC.String != "" {
		out.LargeCommunities = strings.Fields(p.LargeC.String)
	}
	uptime := p.Uptime
	if uptime == "" && p.LastUpdate != nil {
		uptime = p.LastUpdate.String
	}
	if uptime != "" {
		out.AgeSeconds = parseFRRDuration(uptime)
	}
	return out
}

// parseFRRDuration converts FRR's compact uptime strings ("1d02h",
// "00:01:30", "90") into a seconds count. Returns 0 on any
// unrecognised input.
func parseFRRDuration(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.Count(s, ":") == 2 {
		var h, m, sec uint64
		_, err := fscanCount(s, &h, &m, &sec)
		if err == nil {
			return h*3600 + m*60 + sec
		}
	}
	var total, cur uint64
	matched := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			cur = cur*10 + uint64(r-'0')
		case r == 'w' || r == 'W':
			total += cur * 7 * 86400
			cur, matched = 0, true
		case r == 'd' || r == 'D':
			total += cur * 86400
			cur, matched = 0, true
		case r == 'h' || r == 'H':
			total += cur * 3600
			cur, matched = 0, true
		case r == 'm' || r == 'M':
			total += cur * 60
			cur, matched = 0, true
		case r == 's' || r == 'S':
			total += cur
			cur, matched = 0, true
		}
	}
	if matched {
		return total
	}
	var v uint64
	_, err := fscanCount(s, &v)
	if err == nil {
		return v
	}
	return 0
}

// fscanCount parses up to len(dsts) decimal integers from s,
// separated by ':' or whitespace, into the dst pointers. Returns
// the count consumed and a non-nil error if any field was missing.
func fscanCount(s string, dsts ...*uint64) (int, error) {
	consumed := 0
	idx := 0
	for _, dst := range dsts {
		for idx < len(s) && (s[idx] == ':' || s[idx] == ' ') {
			idx++
		}
		if idx >= len(s) {
			return consumed, errParseDuration
		}
		var v uint64
		start := idx
		for idx < len(s) && s[idx] >= '0' && s[idx] <= '9' {
			v = v*10 + uint64(s[idx]-'0')
			idx++
		}
		if start == idx {
			return consumed, errParseDuration
		}
		*dst = v
		consumed++
	}
	return consumed, nil
}

// errParseDuration is returned by fscanCount when an expected field
// is missing or non-numeric.
var errParseDuration = stringErr("parse duration")

// stringErr is a string-backed error type.
type stringErr string

// Error implements the error interface.
func (e stringErr) Error() string { return string(e) }

// splitJSONObjects splits raw into a slice of complete top-level
// JSON object chunks. Brace-counting is string- and escape-aware,
// so braces inside JSON strings never break the split.
func splitJSONObjects(raw []byte) [][]byte {
	var out [][]byte
	depth := 0
	start := -1
	inString := false
	escaped := false
	for i, b := range raw {
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' && inString {
			escaped = true
			continue
		}
		if b == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch b {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, raw[start:i+1])
				start = -1
			}
		}
	}
	return out
}

// frrBGPSummaryAFI is one per-AFI block of an FRR BGP summary.
type frrBGPSummaryAFI struct {
	RouterID string                       `json:"routerId"`
	AS       uint32                       `json:"as"`
	Peers    map[string]frrBGPSummaryPeer `json:"peers"`
}

// frrBGPSummaryRoot is the wrapped multi-AFI envelope shape.
type frrBGPSummaryRoot struct {
	IPv4Unicast *frrBGPSummaryAFI `json:"ipv4Unicast,omitempty"`
	IPv6Unicast *frrBGPSummaryAFI `json:"ipv6Unicast,omitempty"`
}

// frrBGPSummaryPeer is one peer row in an [frrBGPSummaryAFI].
type frrBGPSummaryPeer struct {
	RemoteAs       uint32 `json:"remoteAs"`
	Description    string `json:"description"`
	State          string `json:"state"`
	PeerUptimeMsec uint64 `json:"peerUptimeMsec"`
	PfxRcd         uint64 `json:"pfxRcd"`
	PfxSnt         uint64 `json:"pfxSnt"`
}

// parseFRRBGPSummary decodes one or more FRR `show bgp summary
// json` envelopes into [pb.BGPSummaryParsed]. Wrapped multi-AFI
// and direct single-AFI shapes are both accepted; a chunk with
// an empty peers map is reported as OK with no peer rows.
func parseFRRBGPSummary(raw []byte) Result {
	out := &pb.BGPSummaryParsed{}
	chunks := splitJSONObjects(raw)
	if len(chunks) == 0 {
		parseLog.Warn("frr bgp summary json: no top-level JSON objects found")
		return Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
	decodedAny := false
	for _, chunk := range chunks {
		var root frrBGPSummaryRoot
		if err := json.Unmarshal(chunk, &root); err == nil &&
			(root.IPv4Unicast != nil || root.IPv6Unicast != nil) {
			decodedAny = true
			mergeFRRSummary(out, root.IPv4Unicast, "ipv4-unicast")
			mergeFRRSummary(out, root.IPv6Unicast, "ipv6-unicast")
			continue
		}
		var direct frrBGPSummaryAFI
		if err := json.Unmarshal(chunk, &direct); err != nil {
			parseLog.Warn("frr bgp summary json unmarshal",
				slog.Any("err", err))
			continue
		}
		if direct.Peers == nil {
			continue
		}
		decodedAny = true
		mergeFRRSummary(out, &direct, inferAFILabel(direct.Peers))
	}
	if !decodedAny {
		return Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
	return OK(pb.ParserKind_PARSER_KIND_NATIVE_JSON, proto.Message(out))
}

// inferAFILabel returns "ipv4-unicast" / "ipv6-unicast" / "" based
// on whether every peer key in peers parses as an IPv4 address,
// every key parses as an IPv6 address, or the set is mixed.
func inferAFILabel(peers map[string]frrBGPSummaryPeer) string {
	v4, v6 := false, false
	for ip := range peers {
		if strings.Contains(ip, ":") {
			v6 = true
		} else if strings.Contains(ip, ".") {
			v4 = true
		}
	}
	switch {
	case v4 && !v6:
		return "ipv4-unicast"
	case v6 && !v4:
		return "ipv6-unicast"
	default:
		return ""
	}
}

// mergeFRRSummary appends one AFI block onto out, stamping label as
// each peer row's address_family.
func mergeFRRSummary(out *pb.BGPSummaryParsed, afi *frrBGPSummaryAFI, label string) {
	if afi == nil {
		return
	}
	if out.RouterId == "" {
		out.RouterId = afi.RouterID
	}
	if out.LocalAsn == 0 {
		out.LocalAsn = afi.AS
	}
	for ip, p := range afi.Peers {
		out.Peers = append(out.Peers, &pb.BGPPeer{
			PeerIp:           ip,
			PeerAsn:          p.RemoteAs,
			Description:      p.Description,
			State:            strings.ToLower(p.State),
			UptimeSeconds:    p.PeerUptimeMsec / 1000,
			PrefixesReceived: p.PfxRcd,
			PrefixesAccepted: p.PfxRcd,
			PrefixesSent:     p.PfxSnt,
			AddressFamily:    label,
		})
	}
}
