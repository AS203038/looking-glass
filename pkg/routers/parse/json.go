package parse

import (
	"encoding/json"
	"log"
	"strings"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"google.golang.org/protobuf/proto"
)

// JSONParser normalises vendor-native JSON output into the typed
// `parsed` payload set. Currently the only producer is FRRouting,
// which holds the RIB as structured data in its userland daemon
// and therefore can emit JSON at a fraction of the cost it would
// take JunOS/Arista/IOS-XE to re-serialise text output.
//
// The schema selector (Config.Schema) chooses the input shape:
//
//   - "frr_bgp_route_v1"          for `show bgp … json` (a "routes"
//                                  object keyed by prefix, value =
//                                  list of paths) **or** the
//                                  alternate single-prefix shape
//                                  (`{prefix, paths: []}`).
//   - "frr_bgp_summary_v1"        for `show bgp summary json`
//                                  (wrapped or direct AFI form).
//
// Unknown schema names log and return PARSE_STATUS_TEMPLATE_MISSING
// so operators see a clear, actionable signal in logs.
type JSONParser struct{}

// Name implements [Parser].
func (JSONParser) Name() string { return "native_json" }

// Parse implements [Parser].
func (p JSONParser) Parse(op Op, raw []byte, cfg Config) Result {
	switch op {
	case OpBGPRoute, OpBGPCommunity, OpBGPLargeCommunity, OpBGPASPath:
		return parseFRRBGPPaths(raw)
	case OpBGPSummary:
		return parseFRRBGPSummary(raw)
	default:
		log.Printf("PARSE: native_json parser has no handler for op=%s schema=%q",
			op, cfg.Schema)
		return Missing(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
}

// ---------------------------------------------------------------------------
// FRR `show bgp <prefix|community|aspath> json`
// ---------------------------------------------------------------------------
//
// FRR emits two top-level shapes depending on the query:
//
//  1. **Multi-prefix dump** (community / large-community / aspath /
//     full-table lookups). Top-level is a "routes" map keyed by
//     prefix, value is a list of paths:
//
//	{
//	  "vrfName": "default",
//	  "routerId": "10.0.0.1",
//	  "routes": {
//	     "1.1.1.0/24": [
//	        { "valid": true, "bestpath": {"overall": true},
//	          "med": 0, "locPrf": 100,
//	          "aspath": "65001 65002",                  -- v8.x and earlier
//	          "aspath": {"string": "65001 65002", ...}, -- v9.x and newer
//	          ... }
//	     ]
//	  }
//	}
//
//  2. **Single-prefix detail** (`show bgp vrf X ipv4 unicast 1.1.1.0/24 json`).
//     Top-level is the prefix entry itself:
//
//	{
//	  "prefix": "1.1.1.0/24",
//	  "paths": [
//	     { "aspath": {"string": "...", "segments": [], "length": 2},
//	       "peerId": "10.0.0.2",
//	       "peer": {"peerId": "10.0.0.2", "asn": 65001},
//	       ... }
//	  ]
//	}
//
// FRR's BGP commands return multiple variants when v4 and v6 are
// queried separately; we therefore also accept a concatenated JSON
// stream of two top-level objects (`{...}\n{...}`).

type frrBGPRoutesEnvelope struct {
	VrfName  string                  `json:"vrfName"`
	RouterID string                  `json:"routerId"`
	Routes   map[string][]frrBGPPath `json:"routes"`
}

type frrBGPPath struct {
	Valid bool `json:"valid"`
	// Bestpath is intentionally a RawMessage because FRR emits it
	// in **two** incompatible shapes across versions and commands:
	//   - newer routes-map output: `"bestpath": true`            (bare bool)
	//   - older single-prefix dump: `"bestpath": {"overall": true}` (object)
	// Decoding it as one fixed Go type triggers json.UnmarshalTypeError
	// for the wrong shape and torpedoes the whole envelope decode,
	// which is why structured parsing silently regressed once FRR
	// shipped the bare-bool form. See frrBestpath() for the unified
	// extractor.
	Bestpath json.RawMessage `json:"bestpath"`
	Med      uint32          `json:"med"`
	Metric   uint32          `json:"metric"`
	LocPrf   uint32          `json:"locPrf"`
	// ASPath is FRR's AS-path field. Across versions it appears as
	// `aspath` (v8.x or earlier routes-map; single-prefix detail)
	// and as `path` (v9.x+ routes-map). Path holds the latter as
	// the fallback used by projectFRRPath when ASPath is empty.
	ASPath   json.RawMessage  `json:"aspath"`
	Path     json.RawMessage  `json:"path"`
	Origin   string           `json:"origin"`
	Comm     *frrBGPStringObj `json:"community,omitempty"`
	LargeC   *frrBGPStringObj `json:"largeCommunity,omitempty"`
	Nexthops []frrBGPNexthop  `json:"nexthops"`
	Peer     *frrBGPPeerRef   `json:"peer,omitempty"`
	// Single-prefix lookups carry the peer id at the path level
	// rather than inside a nested `peer` object.
	PeerID string `json:"peerId"`
	// Uptime / lastUpdate carry the path's age. Some FRR versions
	// use `uptime` (string), others a nested `lastUpdate` object.
	Uptime     string            `json:"uptime"`
	LastUpdate *frrBGPLastUpdate `json:"lastUpdate,omitempty"`
}

// frrBestpath extracts the boolean "is best path" flag from FRR's
// dual-shape `bestpath` field. Accepts the bare-bool form
// (`true` / `false`), the object form (`{"overall": true}`), and a
// missing/null field (returns false).
func frrBestpath(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	// Bare bool
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	// Object form: `{"overall": true, ...}`
	var obj struct {
		Overall bool `json:"overall"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Overall
	}
	return false
}


type frrBGPStringObj struct {
	String string `json:"string"`
}

// frrBGPASPathObject decodes FRR's v9.x+ aspath object form. Older
// versions emit a bare JSON string instead; we handle both via the
// json.RawMessage field on frrBGPPath.
type frrBGPASPathObject struct {
	String   string `json:"string"`
	Length   uint32 `json:"length"`
}

type frrBGPNexthop struct {
	IP  string `json:"ip"`
	AFI string `json:"afi"`
}

type frrBGPPeerRef struct {
	PeerID string `json:"peerId"`
	ASN    uint32 `json:"asn"`
}

type frrBGPLastUpdate struct {
	// Epoch is decoded as a json.Number so both integer
	// (`"epoch": 1700000000`) and string (`"epoch": "1700000000"`)
	// forms parse cleanly across FRR versions. We don't currently
	// surface it on the wire — `String` is the canonical age field.
	Epoch  json.Number `json:"epoch"`
	String string      `json:"string"`
}


// parseFRRBGPPaths decodes one or more FRR BGP route-list JSON
// envelopes (concatenated when a template ran v4 and v6 commands
// in sequence) into a [pb.BGPPaths] message.
//
// Empty-result semantics: a successfully-decoded envelope with
// `"routes": {}` (or `"paths": []` on the single-prefix shape) is
// a legitimate router response — the operator's query matched zero
// prefixes. Those decode paths return PARSE_STATUS_OK with an empty
// `paths` slice rather than PARSE_STATUS_PARSE_FAILED, which would
// otherwise force the WebUI/CLI back to raw bytes for every
// no-hit community / aspath / large-community lookup.
//
// PARSE_FAILED is reserved for the genuinely-broken case: no
// top-level JSON objects at all, or every chunk failed to match
// either envelope shape.
func parseFRRBGPPaths(raw []byte) Result {
	paths := &pb.BGPPaths{}
	chunks := splitJSONObjects(raw)
	if len(chunks) == 0 {
		// Some non-empty inputs (vtysh banners + JSON, malformed
		// outputs) may yield no top-level objects; surface that as
		// a parse failure rather than a silent empty payload.
		log.Printf("PARSE: frr bgp json: no top-level JSON objects found")
		return Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
	// decodedAny tracks whether *any* chunk produced a structurally
	// valid envelope. Without this flag we cannot distinguish "the
	// router returned `routes: {}` legitimately" from "no envelope
	// shape matched anywhere" — both leave `paths.Paths` empty.
	decodedAny := false
	for _, chunk := range chunks {
		// Try the multi-prefix `routes` envelope first. A non-nil
		// `Routes` map — even when zero-length — is a valid decode.
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
		// Fall back to the single-prefix `{prefix, paths: []}` shape.
		if tryFRRSinglePrefixJSON(chunk, paths) {
			decodedAny = true
			continue
		}
		log.Printf("PARSE: frr bgp json: neither routes-map nor single-prefix envelope matched")
	}
	if !decodedAny {
		return Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
	return OK(pb.ParserKind_PARSER_KIND_NATIVE_JSON, proto.Message(paths))
}

// tryFRRSinglePrefixJSON handles the alternate shape where the
// top-level object is itself the prefix entry (e.g. `show bgp ipv4
// unicast 1.1.1.0/24 json`). Returns true on a successful decode
// so the caller can move on without logging an error.
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

// extractFRRASPathString returns the dotted-space AS path string
// regardless of whether FRR encoded it as a bare JSON string (v8.x
// and earlier) or a nested object `{"string": "..."}` (v9.x+).
func extractFRRASPathString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try bare string first (most common across all versions when
	// rendering the routes-map envelope).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Then the object form used by single-prefix detail in newer
	// FRR releases.
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
	// MED is reported as `med` on the routes-map envelope but as
	// `metric` on the single-prefix detail form — accept either.
	if p.Med != 0 {
		out.Med = p.Med
	} else {
		out.Med = p.Metric
	}
	if len(p.Nexthops) > 0 {
		out.Nexthop = p.Nexthops[0].IP
	}
	// Peer ID/ASN: prefer the nested object (full info) but fall
	// back to the path-level `peerId` field used by single-prefix
	// detail when the nested object is absent.
	if p.Peer != nil {
		out.PeerIp = p.Peer.PeerID
		out.PeerAsn = p.Peer.ASN
	} else if p.PeerID != "" {
		out.PeerIp = p.PeerID
	}
	// AS-path: try `aspath` first (v8.x, single-prefix detail), then
	// fall back to `path` (v9.x+ routes-map shape). Both go through
	// the same extractor so bare-string / object forms are handled.
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
	// Age: prefer the simple `uptime` string. Fall back to the
	// nested `lastUpdate.string` for newer FRR versions that no
	// longer emit `uptime` at the path level.
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
// "00:01:30", "90") into a seconds count. Best-effort; returns 0
// on any unrecognised format.
func parseFRRDuration(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// HH:MM:SS form
	if strings.Count(s, ":") == 2 {
		var h, m, sec uint64
		_, err := fscanCount(s, &h, &m, &sec)
		if err == nil {
			return h*3600 + m*60 + sec
		}
	}
	// Compound "1w2d3h4m5s" form
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
	// Bare integer = seconds
	var v uint64
	_, err := fscanCount(s, &v)
	if err == nil {
		return v
	}
	return 0
}

// fscanCount is a tiny strconv-free decimal scanner shared by the
// FRR duration parser. It parses dst pointers from s, separated by
// ':' or whitespace, and returns the count consumed plus any error.
func fscanCount(s string, dsts ...*uint64) (int, error) {
	consumed := 0
	idx := 0
	for _, dst := range dsts {
		// skip leading separators
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

// errParseDuration signals an unrecognised duration string.
var errParseDuration = stringErr("parse duration")

// stringErr is a zero-allocation error type used for the duration
// parser's sentinel. (Importing "errors" everywhere here would be
// noise — this is a leaf utility.)
type stringErr string

// Error implements the error interface.
func (e stringErr) Error() string { return string(e) }

// splitJSONObjects splits raw into a slice of complete top-level
// JSON object chunks. FRR's `vtysh` returns one JSON blob per
// command, so when a vendor template runs `show bgp ipv4 ... json`
// and `show bgp ipv6 ... json` in sequence, the SSH output joined
// by gRPC's `joinSSH` (a literal "\n") yields two adjacent objects.
//
// The split is brace-counting aware and respects string literals
// + escape sequences so an `{` inside a JSON string never breaks
// it. A trailing partial / non-object suffix is silently dropped.
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

// ---------------------------------------------------------------------------
// FRR `show bgp summary json`
// ---------------------------------------------------------------------------
//
// FRR's summary command emits two distinct shapes depending on
// whether the query was AFI-scoped:
//
//  1. **Wrapped multi-AFI** (`vtysh -c 'show bgp summary json'`):
//
//	{
//	  "ipv4Unicast": {
//	    "routerId": "10.0.0.1", "as": 65000,
//	    "peers": { "10.0.0.2": {...} }
//	  },
//	  "ipv6Unicast": { ... }
//	}
//
//  2. **Direct single-AFI** (`vtysh -c 'show bgp ipv4 unicast summary json'`
//     — the form our bundled template uses, fanned-out per-AFI):
//
//	{
//	  "routerId": "10.0.0.1", "as": 65000,
//	  "vrfId": 0, "vrfName": "default",
//	  "peers": { "10.0.0.2": {...} }
//	}
//
// The parser accepts both shapes per chunk: wrapped is unmarshalled
// first; if neither `ipv4Unicast` nor `ipv6Unicast` is present we
// fall through to the direct form and derive the AFI label from
// the peer-address family (v4 vs v6) of the rows themselves.

type frrBGPSummaryAFI struct {
	RouterID string                       `json:"routerId"`
	AS       uint32                       `json:"as"`
	Peers    map[string]frrBGPSummaryPeer `json:"peers"`
}

type frrBGPSummaryRoot struct {
	IPv4Unicast *frrBGPSummaryAFI `json:"ipv4Unicast,omitempty"`
	IPv6Unicast *frrBGPSummaryAFI `json:"ipv6Unicast,omitempty"`
}

type frrBGPSummaryPeer struct {
	RemoteAs       uint32 `json:"remoteAs"`
	Description    string `json:"description"`
	State          string `json:"state"`
	PeerUptimeMsec uint64 `json:"peerUptimeMsec"`
	PfxRcd         uint64 `json:"pfxRcd"`
	PfxSnt         uint64 `json:"pfxSnt"`
}

// parseFRRBGPSummary decodes one or more FRR `show bgp summary
// json` envelopes into a [pb.BGPSummaryParsed]. When a template
// issued both an IPv4 and an IPv6 summary in sequence the joined
// output is two top-level objects; we merge them with the AFI
// label attached to each peer row.
//
// Empty-result semantics mirror [parseFRRBGPPaths]: a router with
// zero configured peers in the queried AFI legitimately returns
// `"peers": {}` and we project that as PARSE_STATUS_OK with an
// empty `peers` slice rather than torpedoing the structured-view
// path. PARSE_FAILED is reserved for the case where *no* chunk
// matched either envelope shape.
func parseFRRBGPSummary(raw []byte) Result {
	out := &pb.BGPSummaryParsed{}
	chunks := splitJSONObjects(raw)
	if len(chunks) == 0 {
		log.Printf("PARSE: frr bgp summary json: no top-level JSON objects found")
		return Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
	// decodedAny: any chunk that structurally matched the wrapped
	// or direct envelope shape — even with zero peers — counts.
	decodedAny := false
	for _, chunk := range chunks {
		// First try the wrapped multi-AFI form.
		var root frrBGPSummaryRoot
		if err := json.Unmarshal(chunk, &root); err == nil &&
			(root.IPv4Unicast != nil || root.IPv6Unicast != nil) {
			decodedAny = true
			mergeFRRSummary(out, root.IPv4Unicast, "ipv4-unicast")
			mergeFRRSummary(out, root.IPv6Unicast, "ipv6-unicast")
			continue
		}
		// Fall back to the direct single-AFI form (this is what
		// `show bgp [vrf X] {ipv4|ipv6} unicast summary json`
		// actually emits — the form our bundled template uses).
		var direct frrBGPSummaryAFI
		if err := json.Unmarshal(chunk, &direct); err != nil {
			log.Printf("PARSE: frr bgp summary json unmarshal: %v", err)
			continue
		}
		if direct.Peers == nil {
			continue
		}
		// `peers: {}` is a legitimate zero-peer response — the map
		// is non-nil even when empty, which is the discriminator
		// we use here.
		decodedAny = true
		mergeFRRSummary(out, &direct, inferAFILabel(direct.Peers))
	}
	if !decodedAny {
		return Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON)
	}
	return OK(pb.ParserKind_PARSER_KIND_NATIVE_JSON, proto.Message(out))
}

// inferAFILabel inspects a peers map and returns "ipv4-unicast"
// when every peer key looks like an IPv4 address, "ipv6-unicast"
// when every peer key looks like an IPv6 address, or the empty
// string when the map is mixed/ambiguous. Used only for the direct
// per-AFI summary form, which carries no explicit AFI label of its
// own.
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

// mergeFRRSummary copies one AFI block into the output summary,
// stamping the address-family label on each peer row.
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
			PrefixesAccepted: p.PfxRcd, // FRR reports a single received-and-accepted count.
			PrefixesSent:     p.PfxSnt,
			AddressFamily:    label,
		})
	}
}
