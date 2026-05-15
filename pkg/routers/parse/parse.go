// Package parse turns the raw, vendor-formatted output captured by
// [utils.SSHExec] into the typed `parsed` payloads carried alongside
// every operation response in the gRPC contract.
//
// The package is policy-light by design: it exposes a [Parser]
// interface that vendor-template authors select per-command via
// the YAML schema's `parser:` field, plus three concrete
// implementations covering the three pipes the project supports:
//
//   - [RawParser]    — no parsing; emits status DISABLED so the
//                       wire response carries only the verbatim
//                       router bytes. The default.
//   - [TextFSMParser] — applies a TextFSM template (typically
//                       sourced from networktocode/ntc-templates)
//                       to plain CLI output. The recommended
//                       default for every vendor where router CPU
//                       is the bottleneck.
//   - [JSONParser]    — normalises a vendor's native-JSON output
//                       (`vtysh … json`, `| display json`, `| json`)
//                       into the same typed payload. Opt-in
//                       per-template; reserved for routers where
//                       the JSON path is known to be cheap.
//
// Parser implementations always return the operation-specific
// typed payload via the [Result] envelope. Callers (the gRPC
// handlers in pkg/http/grpc) project that envelope onto the
// corresponding *Response message's `parsed`, `parser_kind` and
// `parse_status` fields.
//
// A failed parse never propagates as a Go error to the RPC caller:
// the raw `result` bytes are always authoritative, and the parser
// layer is additive. Parsers that cannot extract structure log
// server-side and fall through to PARSE_STATUS_PARSE_FAILED with a
// nil payload — the WebUI and CLI then render the raw output
// exactly as they would have before this package existed.
package parse

import (
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"google.golang.org/protobuf/proto"
)

// Op identifies the operation being parsed. Parsers fan out on
// this so a single Parser instance can serve every command in a
// vendor template (e.g. one TextFSMParser instance handles ping,
// traceroute and the four BGP commands by selecting different
// embedded `.textfsm` files).
type Op string

// Operation identifiers. These mirror the YAML template keys so
// the dispatcher can route on the string form directly without an
// extra mapping table.
const (
	OpPing              Op = "ping"
	OpTraceroute        Op = "traceroute"
	OpBGPRoute          Op = "bgp.route"
	OpBGPCommunity      Op = "bgp.community"
	OpBGPLargeCommunity Op = "bgp.largecommunity"
	OpBGPASPath         Op = "bgp.aspath"
	OpBGPSummary        Op = "bgp.summary"
)

// Result is the envelope returned by every [Parser.Parse] call.
// Payload carries the operation-specific typed message
// (e.g. *pb.PingStats for OpPing) or nil when the parser produced
// no useful structure. Kind names the parser pipe that ran; Status
// describes the outcome. The gRPC layer projects this onto the
// response message's parsed / parser_kind / parse_status fields.
type Result struct {
	// Payload is the typed *Parsed message for the operation, or
	// nil when parsing was skipped or failed. The concrete type
	// depends on Op:
	//
	//   OpPing             → *pb.PingStats
	//   OpTraceroute       → *pb.TracerouteParsed
	//   OpBGPSummary       → *pb.BGPSummaryParsed
	//   OpBGPRoute,
	//   OpBGPCommunity,
	//   OpBGPLargeCommunity,
	//   OpBGPASPath        → *pb.BGPPaths
	Payload proto.Message
	// Kind names the parser pipe that produced Payload, or
	// PARSER_KIND_UNSPECIFIED when no parser ran.
	Kind pb.ParserKind
	// Status reports the outcome of the parser attempt; see
	// [pb.ParseStatus] for semantics.
	Status pb.ParseStatus
}

// Disabled is the canonical "no parser was configured" Result.
// Returned by [RawParser] and by the dispatcher when a vendor
// template's command entry omits `parser:` or sets it to "raw".
func Disabled() Result {
	return Result{
		Payload: nil,
		Kind:    pb.ParserKind_PARSER_KIND_UNSPECIFIED,
		Status:  pb.ParseStatus_PARSE_STATUS_DISABLED,
	}
}

// Failed is the canonical "parser ran but produced nothing useful"
// Result. Kind is preserved so debug surfaces can tell *which*
// parser failed; Payload is nil so clients fall back to the raw
// `result` bytes.
func Failed(kind pb.ParserKind) Result {
	return Result{
		Payload: nil,
		Kind:    kind,
		Status:  pb.ParseStatus_PARSE_STATUS_PARSE_FAILED,
	}
}

// Missing is returned when a parser was configured but its
// template / schema asset could not be located at startup. This
// is a packaging defect that the operator cannot recover from at
// runtime; the gRPC handler still serves `result` so the user-
// facing operation continues to work.
func Missing(kind pb.ParserKind) Result {
	return Result{
		Payload: nil,
		Kind:    kind,
		Status:  pb.ParseStatus_PARSE_STATUS_TEMPLATE_MISSING,
	}
}

// OK wraps a successful Payload into a Result with the supplied
// Kind. Convenience constructor used by every parser
// implementation that produced structured output.
func OK(kind pb.ParserKind, payload proto.Message) Result {
	return Result{
		Payload: payload,
		Kind:    kind,
		Status:  pb.ParseStatus_PARSE_STATUS_OK,
	}
}

// Parser turns raw command output into a typed payload. A single
// Parser instance may serve every operation in a vendor template;
// op carries the routing hint, cfg carries optional parser-private
// configuration loaded from the YAML template (TextFSM template
// name, JSON schema selector, …).
//
// Implementations must never modify raw; the byte slice is shared
// with the caller and may be cached for the response.
type Parser interface {
	// Name returns a stable, human-readable identifier for the
	// parser, used in log lines and the response's parser_kind
	// derivation.
	Name() string
	// Parse runs the parser against raw and returns a [Result].
	// The function must never return a Go error: parsing failures
	// are reported via Result.Status, not via err, so the calling
	// RPC handler always has unambiguous "send raw + status" path.
	Parse(op Op, raw []byte, cfg Config) Result
}

// Config carries parser-private settings that the YAML template
// passes through to [Parser.Parse]. The same struct is used by
// every parser kind; unused fields are left zero-valued. Adding
// fields here is the supported way to extend the schema without
// touching the Parser interface.
type Config struct {
	// Template names the TextFSM template asset, relative to the
	// `textfsm/` directory (e.g. "arista_eos_show_bgp_summary").
	// Used only by [TextFSMParser].
	Template string
	// Schema names the JSON schema selector for native-JSON
	// parsers (e.g. "frr_show_bgp_route_v1"). Used only by
	// [JSONParser].
	Schema string
}

// RawParser is the no-op parser. It returns [Disabled] for every
// invocation so that vendor templates which decline to parse keep
// the existing "raw bytes only" wire shape.
type RawParser struct{}

// Name implements [Parser].
func (RawParser) Name() string { return "raw" }

// Parse implements [Parser] by returning [Disabled] unconditionally.
func (RawParser) Parse(_ Op, _ []byte, _ Config) Result { return Disabled() }
