// Package parse converts raw, vendor-formatted command output into
// the typed `parsed` payloads carried on every operation response.
//
// The package exposes a [Parser] interface and three concrete
// implementations: [RawParser] (no parsing), [TextFSMParser]
// (gotextfsm against plain CLI output), and [JSONParser] (vendor-
// native JSON). [BuiltinParser] in builtin.go is a small hand-rolled
// parser covering Linux iputils ping and traceroute.
//
// Parsers always return a [Result] envelope; they never return a Go
// error. Parse failures are reported via Result.Status so the gRPC
// layer always has an unambiguous "send raw + status" path.
package parse

import (
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"google.golang.org/protobuf/proto"
)

// Op identifies the operation being parsed.
type Op string

// Operation identifiers; mirror the YAML template keys.
const (
	OpPing                    Op = "ping"
	OpTraceroute              Op = "traceroute"
	OpBGPRoute                Op = "bgp.route"
	OpBGPCommunity            Op = "bgp.community"
	OpBGPLargeCommunity       Op = "bgp.largecommunity"
	OpBGPASPath               Op = "bgp.aspath"
	OpBGPSummary              Op = "bgp.summary"
	OpBGPPeerRoutesReceived   Op = "bgp.peer_routes.received"
	OpBGPPeerRoutesAccepted   Op = "bgp.peer_routes.accepted"
	OpBGPPeerRoutesRejected   Op = "bgp.peer_routes.rejected"
	OpBGPPeerRoutesAdvertised Op = "bgp.peer_routes.advertised"
)

// Result is the envelope returned by every [Parser.Parse] call.
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
	//   OpBGPASPath,
	//   OpBGPPeerRoutesReceived,
	//   OpBGPPeerRoutesAccepted,
	//   OpBGPPeerRoutesRejected,
	//   OpBGPPeerRoutesAdvertised → *pb.BGPPaths
	Payload proto.Message
	// Kind names the parser pipe that produced Payload.
	Kind pb.ParserKind
	// Status reports the outcome of the parser attempt.
	Status pb.ParseStatus
}

// Disabled returns a [Result] with status PARSE_STATUS_DISABLED and
// no payload, used when no parser is configured for the operation.
func Disabled() Result {
	return Result{
		Payload: nil,
		Kind:    pb.ParserKind_PARSER_KIND_UNSPECIFIED,
		Status:  pb.ParseStatus_PARSE_STATUS_DISABLED,
	}
}

// Failed returns a [Result] with status PARSE_STATUS_PARSE_FAILED
// and no payload, preserving kind so clients can identify which
// parser failed.
func Failed(kind pb.ParserKind) Result {
	return Result{
		Payload: nil,
		Kind:    kind,
		Status:  pb.ParseStatus_PARSE_STATUS_PARSE_FAILED,
	}
}

// Missing returns a [Result] with status
// PARSE_STATUS_TEMPLATE_MISSING and no payload, used when a parser
// was configured but its template or schema could not be located.
func Missing(kind pb.ParserKind) Result {
	return Result{
		Payload: nil,
		Kind:    kind,
		Status:  pb.ParseStatus_PARSE_STATUS_TEMPLATE_MISSING,
	}
}

// OK returns a [Result] with status PARSE_STATUS_OK wrapping a
// successful payload of the given kind.
func OK(kind pb.ParserKind, payload proto.Message) Result {
	return Result{
		Payload: payload,
		Kind:    kind,
		Status:  pb.ParseStatus_PARSE_STATUS_OK,
	}
}

// Parser converts raw command output into a typed payload. A single
// instance may serve every operation in a vendor template;
// implementations route on op and consume cfg for parser-private
// settings. Parse must never return a Go error: failures are
// reported via Result.Status.
type Parser interface {
	// Name returns a stable, human-readable identifier for the parser.
	Name() string
	// Parse runs the parser against raw and returns a [Result].
	Parse(op Op, raw []byte, cfg Config) Result
}

// Config carries parser-private settings supplied by the YAML
// template. Unused fields are left zero-valued.
type Config struct {
	// Template names the TextFSM template asset, relative to the
	// `textfsm/` directory (used by [TextFSMParser]).
	Template string
	// Schema names the JSON schema selector (used by [JSONParser]).
	Schema string
}

// RawParser is the no-op [Parser] returning [Disabled] for every
// invocation.
type RawParser struct{}

// Name implements [Parser].
func (RawParser) Name() string { return "raw" }

// Parse implements [Parser] by returning [Disabled] unconditionally.
func (RawParser) Parse(_ Op, _ []byte, _ Config) Result { return Disabled() }
