package parse

import (
	"testing"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

// TestRawParser verifies that the no-op parser always reports
// DISABLED with no payload, regardless of operation or input. This
// is the contract every gRPC handler relies on for the "no
// behaviour change" guarantee of the schema-only PR.
func TestRawParser(t *testing.T) {
	var p Parser = RawParser{}
	if p.Name() != "raw" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "raw")
	}

	cases := []struct {
		name string
		op   Op
		raw  []byte
	}{
		{"ping empty", OpPing, nil},
		{"ping data", OpPing, []byte("64 bytes from 1.1.1.1: time=2.1 ms\n")},
		{"bgp.route empty", OpBGPRoute, []byte{}},
		{"bgp.summary data", OpBGPSummary, []byte("Neighbor V AS")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := p.Parse(tc.op, tc.raw, Config{})
			if r.Payload != nil {
				t.Errorf("Payload = %v, want nil", r.Payload)
			}
			if r.Kind != pb.ParserKind_PARSER_KIND_UNSPECIFIED {
				t.Errorf("Kind = %v, want UNSPECIFIED", r.Kind)
			}
			if r.Status != pb.ParseStatus_PARSE_STATUS_DISABLED {
				t.Errorf("Status = %v, want DISABLED", r.Status)
			}
		})
	}
}

// TestResultHelpers verifies the canonical Result constructors emit
// the exact Kind/Status pairs the gRPC layer projects onto the wire.
func TestResultHelpers(t *testing.T) {
	if r := Disabled(); r.Kind != pb.ParserKind_PARSER_KIND_UNSPECIFIED ||
		r.Status != pb.ParseStatus_PARSE_STATUS_DISABLED || r.Payload != nil {
		t.Errorf("Disabled() = %+v", r)
	}
	if r := Missing(pb.ParserKind_PARSER_KIND_TEXTFSM); r.Kind != pb.ParserKind_PARSER_KIND_TEXTFSM ||
		r.Status != pb.ParseStatus_PARSE_STATUS_TEMPLATE_MISSING || r.Payload != nil {
		t.Errorf("Missing() = %+v", r)
	}
	if r := Failed(pb.ParserKind_PARSER_KIND_NATIVE_JSON); r.Kind != pb.ParserKind_PARSER_KIND_NATIVE_JSON ||
		r.Status != pb.ParseStatus_PARSE_STATUS_PARSE_FAILED || r.Payload != nil {
		t.Errorf("Failed() = %+v", r)
	}
	payload := &pb.PingStats{PacketsSent: 5, PacketsReceived: 5}
	if r := OK(pb.ParserKind_PARSER_KIND_BUILTIN, payload); r.Kind != pb.ParserKind_PARSER_KIND_BUILTIN ||
		r.Status != pb.ParseStatus_PARSE_STATUS_OK || r.Payload != payload {
		t.Errorf("OK() = %+v", r)
	}
}
