package parse

import (
	"testing"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

// Canonical Linux iputils ping output, IPv4, 5 probes, no loss.
const linuxPingIPv4Sample = `PING 1.1.1.1 (1.1.1.1) from 10.0.0.1 : 56(84) bytes of data.
64 bytes from 1.1.1.1: icmp_seq=1 ttl=58 time=2.10 ms
64 bytes from 1.1.1.1: icmp_seq=2 ttl=58 time=2.05 ms
64 bytes from 1.1.1.1: icmp_seq=3 ttl=58 time=2.34 ms
64 bytes from 1.1.1.1: icmp_seq=4 ttl=58 time=2.12 ms
64 bytes from 1.1.1.1: icmp_seq=5 ttl=58 time=2.08 ms

--- 1.1.1.1 ping statistics ---
5 packets transmitted, 5 received, 0% packet loss, time 4006ms
rtt min/avg/max/mdev = 2.052/2.138/2.340/0.106 ms
`

// 100% loss form — no rtt summary line is emitted.
const linuxPingLossSample = `PING 10.255.255.1 (10.255.255.1) from 10.0.0.1 : 56(84) bytes of data.

--- 10.255.255.1 ping statistics ---
5 packets transmitted, 0 received, 100% packet loss, time 4081ms
`

// IPv6 form — different header layout, same stats block.
const linuxPingIPv6Sample = `PING 2606:4700:4700::1111(2606:4700:4700::1111) from 2001:db8::1 : 56 data bytes
64 bytes from 2606:4700:4700::1111: icmp_seq=1 ttl=58 time=2.10 ms

--- 2606:4700:4700::1111 ping statistics ---
5 packets transmitted, 5 received, 0% packet loss, time 4006ms
rtt min/avg/max/mdev = 2.052/2.138/2.340/0.106 ms
`

// TestParseLinuxPingIPv4 covers the dominant success path: full
// stats + RTT block, with the optional source-address capture.
func TestParseLinuxPingIPv4(t *testing.T) {
	s := parseLinuxPing([]byte(linuxPingIPv4Sample))
	if s == nil {
		t.Fatal("parseLinuxPing returned nil for a healthy sample")
	}
	if s.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", s.Target)
	}
	if s.Source != "10.0.0.1" {
		t.Errorf("Source = %q, want 10.0.0.1", s.Source)
	}
	if s.PacketsSent != 5 || s.PacketsReceived != 5 {
		t.Errorf("packets: sent=%d recv=%d, want 5/5", s.PacketsSent, s.PacketsReceived)
	}
	if s.LossPct != 0 {
		t.Errorf("LossPct = %v, want 0", s.LossPct)
	}
	if s.RttMinMs == 0 || s.RttAvgMs == 0 || s.RttMaxMs == 0 || s.RttMdevMs == 0 {
		t.Errorf("RTT fields not parsed: min=%v avg=%v max=%v mdev=%v",
			s.RttMinMs, s.RttAvgMs, s.RttMaxMs, s.RttMdevMs)
	}
}

// TestParseLinuxPing100PctLoss covers the no-replies path: stats
// block populated, RTT block missing.
func TestParseLinuxPing100PctLoss(t *testing.T) {
	s := parseLinuxPing([]byte(linuxPingLossSample))
	if s == nil {
		t.Fatal("parseLinuxPing returned nil for a 100% loss sample")
	}
	if s.PacketsSent != 5 || s.PacketsReceived != 0 {
		t.Errorf("packets: sent=%d recv=%d, want 5/0", s.PacketsSent, s.PacketsReceived)
	}
	if s.LossPct != 100 {
		t.Errorf("LossPct = %v, want 100", s.LossPct)
	}
	if s.RttAvgMs != 0 {
		t.Errorf("RttAvgMs = %v, want 0 on no-reply sample", s.RttAvgMs)
	}
}

// TestParseLinuxPingIPv6 covers the v6 header variant where the
// address is glued to the parenthesised duplicate.
func TestParseLinuxPingIPv6(t *testing.T) {
	s := parseLinuxPing([]byte(linuxPingIPv6Sample))
	if s == nil {
		t.Fatal("parseLinuxPing returned nil for v6 sample")
	}
	if s.Target != "2606:4700:4700::1111" {
		t.Errorf("Target = %q, want 2606:4700:4700::1111", s.Target)
	}
	if s.Source != "2001:db8::1" {
		t.Errorf("Source = %q, want 2001:db8::1", s.Source)
	}
	if s.PacketsReceived != 5 {
		t.Errorf("PacketsReceived = %d, want 5", s.PacketsReceived)
	}
}

// TestParseLinuxPingEmpty returns nil for empty / non-ping input so
// the gRPC handler falls through to PARSE_FAILED + raw bytes.
func TestParseLinuxPingEmpty(t *testing.T) {
	if s := parseLinuxPing([]byte("")); s != nil {
		t.Errorf("empty input parsed to %+v, want nil", s)
	}
	if s := parseLinuxPing([]byte("traceroute to 1.1.1.1\n")); s != nil {
		t.Errorf("traceroute input parsed to %+v, want nil", s)
	}
}

const linuxTracerouteSample = `traceroute to 1.1.1.1 (1.1.1.1) from 10.0.0.1, 30 hops max, 60 byte packets
 1  gw.example.com (10.0.0.254)  0.234 ms
 2  *
 3  one.one.one.one (1.1.1.1)  10.123 ms
`

// FRR template traceroute output with `-e --back --mtu`: hop 1 is
// a star with an MTU annotation; hops 2–3 carry a back-path quote
// "'-N'" between the (IP) and the RTT. Regression sample.
const frrTracerouteSample = `traceroute to 1.1.1.1 (1.1.1.1), 30 hops max, 65000 byte packets
 1  * F=1500
 2  172.68.180.37 (172.68.180.37) '-5'  0.885 ms
 3  one.one.one.one (1.1.1.1) '-6'  0.867 ms
`

// FRR template traceroute output without back-path quotes (probe
// never traversed an AS boundary) but with star hops scattered
// throughout the path. Regression sample.
const frrTracerouteMixedSample = `traceroute to 31.13.72.36 (31.13.72.36), 30 hops max, 65000 byte packets
 1  * F=1500
 2  ae1-358.kis-dlr.obe.net (195.128.254.29)  0.225 ms
 3  163.77.146.54 (163.77.146.54)  0.352 ms
 4  *
 5  psw03.arn2.tfbnw.net (74.119.79.149)  0.211 ms
 6  *
 7  edge-star-mini-shv-01-arn2.facebook.com (31.13.72.36)  0.204 ms
`

// TestParseLinuxTraceroute covers all three hop shapes in one go:
// resolved hostname, full timeout, and resolved hostname for the
// destination row.
func TestParseLinuxTraceroute(t *testing.T) {
	tp := parseLinuxTraceroute([]byte(linuxTracerouteSample))
	if tp == nil {
		t.Fatal("parseLinuxTraceroute returned nil")
	}
	if tp.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", tp.Target)
	}
	if tp.Source != "10.0.0.1" {
		t.Errorf("Source = %q, want 10.0.0.1", tp.Source)
	}
	if got := len(tp.Hops); got != 3 {
		t.Fatalf("len(Hops) = %d, want 3", got)
	}
	// Hop 1: resolved
	if tp.Hops[0].Ttl != 1 || len(tp.Hops[0].Probes) != 1 ||
		tp.Hops[0].Probes[0].Ip != "10.0.0.254" ||
		tp.Hops[0].Probes[0].Hostname != "gw.example.com" {
		t.Errorf("hop 1: %+v", tp.Hops[0])
	}
	// Hop 2: timeout (one synthetic empty probe)
	if tp.Hops[1].Ttl != 2 || len(tp.Hops[1].Probes) != 1 ||
		tp.Hops[1].Probes[0].Ip != "" || tp.Hops[1].Probes[0].RttMs != 0 {
		t.Errorf("hop 2 (timeout): %+v", tp.Hops[1])
	}
	// Hop 3: destination
	if tp.Hops[2].Ttl != 3 || tp.Hops[2].Probes[0].Ip != "1.1.1.1" ||
		tp.Hops[2].Probes[0].RttMs == 0 {
		t.Errorf("hop 3: %+v", tp.Hops[2])
	}
}

// TestParseLinuxTracerouteFRRDecorations covers FRR's
// `-e --back --mtu` decoration: an MTU-only "* F=NNNN" line, and
// hops with a back-path quote "'-N'" between (IP) and RTT.
func TestParseLinuxTracerouteFRRDecorations(t *testing.T) {
	tp := parseLinuxTraceroute([]byte(frrTracerouteSample))
	if tp == nil {
		t.Fatal("parseLinuxTraceroute returned nil")
	}
	if tp.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", tp.Target)
	}
	if got := len(tp.Hops); got != 3 {
		t.Fatalf("len(Hops) = %d, want 3", got)
	}
	// Hop 1: "* F=1500" → one synthetic timeout probe, no IP.
	if tp.Hops[0].Ttl != 1 || len(tp.Hops[0].Probes) != 1 ||
		tp.Hops[0].Probes[0].Ip != "" {
		t.Errorf("hop 1 (timeout+F): %+v", tp.Hops[0])
	}
	// Hop 2: back-path "'-5'" between (IP) and "0.885 ms" must
	// not break the probe match.
	if tp.Hops[1].Ttl != 2 || len(tp.Hops[1].Probes) != 1 ||
		tp.Hops[1].Probes[0].Ip != "172.68.180.37" ||
		tp.Hops[1].Probes[0].Hostname != "172.68.180.37" ||
		tp.Hops[1].Probes[0].RttMs == 0 {
		t.Errorf("hop 2 (back-path): %+v", tp.Hops[1])
	}
	// Hop 3: same shape, real reverse-DNS name.
	if tp.Hops[2].Ttl != 3 || tp.Hops[2].Probes[0].Hostname != "one.one.one.one" ||
		tp.Hops[2].Probes[0].Ip != "1.1.1.1" || tp.Hops[2].Probes[0].RttMs == 0 {
		t.Errorf("hop 3: %+v", tp.Hops[2])
	}
}

// TestParseLinuxTracerouteFRRMixed covers the FRR template path
// where some hops carry no back-path quote (probe didn't cross an
// AS boundary) and `*`-timeout hops are scattered through.
func TestParseLinuxTracerouteFRRMixed(t *testing.T) {
	tp := parseLinuxTraceroute([]byte(frrTracerouteMixedSample))
	if tp == nil {
		t.Fatal("parseLinuxTraceroute returned nil")
	}
	if got := len(tp.Hops); got != 7 {
		t.Fatalf("len(Hops) = %d, want 7", got)
	}
	// Hop 1: "* F=1500"
	if tp.Hops[0].Probes[0].Ip != "" {
		t.Errorf("hop 1: %+v", tp.Hops[0])
	}
	// Hop 2: resolved
	if tp.Hops[1].Probes[0].Ip != "195.128.254.29" {
		t.Errorf("hop 2: %+v", tp.Hops[1])
	}
	// Hop 4: pure star
	if len(tp.Hops[3].Probes) != 1 || tp.Hops[3].Probes[0].Ip != "" ||
		tp.Hops[3].Probes[0].RttMs != 0 {
		t.Errorf("hop 4 (star): %+v", tp.Hops[3])
	}
	// Hop 7: destination
	if tp.Hops[6].Probes[0].Hostname != "edge-star-mini-shv-01-arn2.facebook.com" {
		t.Errorf("hop 7: %+v", tp.Hops[6])
	}
}

// TestBuiltinParserDispatch covers the Parse() switch fan-out and
// confirms the kind/status projection.
func TestBuiltinParserDispatch(t *testing.T) {
	p := BuiltinParser{}
	if p.Name() != "builtin" {
		t.Fatalf("Name = %q", p.Name())
	}

	r := p.Parse(OpPing, []byte(linuxPingIPv4Sample), Config{})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK ||
		r.Kind != pb.ParserKind_PARSER_KIND_BUILTIN || r.Payload == nil {
		t.Errorf("ping OK: %+v", r)
	}

	r = p.Parse(OpPing, []byte(""), Config{})
	if r.Status != pb.ParseStatus_PARSE_STATUS_PARSE_FAILED || r.Payload != nil {
		t.Errorf("ping empty should fail: %+v", r)
	}

	r = p.Parse(OpTraceroute, []byte(linuxTracerouteSample), Config{})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK || r.Payload == nil {
		t.Errorf("traceroute OK: %+v", r)
	}

	r = p.Parse(OpBGPRoute, []byte("anything"), Config{})
	if r.Status != pb.ParseStatus_PARSE_STATUS_PARSE_FAILED {
		t.Errorf("builtin should not handle bgp.route: %+v", r)
	}
}
