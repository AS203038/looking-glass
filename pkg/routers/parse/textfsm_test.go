package parse

import (
	"testing"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

// TestTextFSMTemplatesLoad asserts that every bundled `.textfsm`
// template parses successfully via [resolveTemplate].
func TestTextFSMTemplatesLoad(t *testing.T) {
	files, err := templates.ReadDir("textfsm")
	if err != nil {
		t.Fatalf("read embedded textfsm dir: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no embedded textfsm templates found — packaging defect")
	}
	for _, f := range files {
		name := f.Name()
		// Trim extension to feed [resolveTemplate].
		base := name
		for _, ext := range []string{".textfsm", ".tfsm"} {
			if l := len(ext); len(base) > l && base[len(base)-l:] == ext {
				base = base[:len(base)-l]
				break
			}
		}
		t.Run(name, func(t *testing.T) {
			if _, err := resolveTemplate(base); err != nil {
				t.Errorf("template %q failed to load: %v", name, err)
			}
		})
	}
}

// TestTextFSMParserMissing covers the template-missing error path.
func TestTextFSMParserMissing(t *testing.T) {
	r := TextFSMParser{}.Parse(OpPing, []byte("anything"),
		Config{Template: "does_not_exist"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_TEMPLATE_MISSING {
		t.Errorf("Status = %v, want TEMPLATE_MISSING", r.Status)
	}
	if r.Kind != pb.ParserKind_PARSER_KIND_TEXTFSM {
		t.Errorf("Kind = %v, want TEXTFSM", r.Kind)
	}
}

// TestTextFSMParserAristaPing verifies the Arista ping template
// against the shared iputils ping sample.
func TestTextFSMParserAristaPing(t *testing.T) {
	r := TextFSMParser{}.Parse(OpPing, []byte(linuxPingIPv4Sample),
		Config{Template: "arista_eos_ping"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	stats, ok := r.Payload.(*pb.PingStats)
	if !ok {
		t.Fatalf("Payload type = %T, want *pb.PingStats", r.Payload)
	}
	if stats.Target != "1.1.1.1" || stats.PacketsSent != 5 || stats.PacketsReceived != 5 {
		t.Errorf("stats: %+v", stats)
	}
}

const ciscoIOSPingSample = `Type escape sequence to abort.
Sending 5, 100-byte ICMP Echos to 1.1.1.1, timeout is 2 seconds:
Packet sent with a source address of 10.0.0.1
!!!!!
Success rate is 100 percent (5/5), round-trip min/avg/max = 1/2/4 ms
`

// TestTextFSMParserCiscoIOSPing exercises the Cisco success-rate
// ping format (no loss_pct capture; loss_pct is left zero).
func TestTextFSMParserCiscoIOSPing(t *testing.T) {
	r := TextFSMParser{}.Parse(OpPing, []byte(ciscoIOSPingSample),
		Config{Template: "cisco_ios_ping"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	stats, ok := r.Payload.(*pb.PingStats)
	if !ok {
		t.Fatalf("Payload type = %T, want *pb.PingStats", r.Payload)
	}
	if stats.Target != "1.1.1.1" || stats.Source != "10.0.0.1" {
		t.Errorf("target/source: %+v", stats)
	}
	if stats.PacketsSent != 5 || stats.PacketsReceived != 5 {
		t.Errorf("packets sent=%d recv=%d, want 5/5",
			stats.PacketsSent, stats.PacketsReceived)
	}
	if stats.RttMinMs != 1 || stats.RttAvgMs != 2 || stats.RttMaxMs != 4 {
		t.Errorf("rtt: %+v", stats)
	}
}

const ciscoIOSTracerouteSample = `Type escape sequence to abort.
Tracing the route to one.one.one.one (1.1.1.1)
VRF info: (vrf in name/id, vrf out name/id)
  1 10.0.0.254 0 msec 0 msec 0 msec
  2 192.0.2.1 [AS 64500] 1 msec 1 msec *
  3 * * *
  4 one.one.one.one (1.1.1.1) 8 msec 8 msec 8 msec
`

// TestTextFSMParserCiscoIOSTraceroute covers the multi-probe-per-line
// shape with mixed responses and timeouts.
func TestTextFSMParserCiscoIOSTraceroute(t *testing.T) {
	r := TextFSMParser{}.Parse(OpTraceroute, []byte(ciscoIOSTracerouteSample),
		Config{Template: "cisco_ios_traceroute"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	tp, ok := r.Payload.(*pb.TracerouteParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if tp.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", tp.Target)
	}
	if got := len(tp.Hops); got != 4 {
		t.Fatalf("len(Hops) = %d, want 4", got)
	}
	// Hop 1 — three resolved probes, same IP. The sample's RTTs
	// are all "0 msec" (the first hop's transmit-to-LAN latency
	// rounds down on a Cisco); we check that none of those three
	// slots was trimmed by the projector's empty-slot logic.
	if got := len(tp.Hops[0].Probes); got != 3 {
		t.Errorf("hop1: probes = %d, want 3", got)
	}
	if tp.Hops[0].Probes[0].Ip != "10.0.0.254" {
		t.Errorf("hop1 probe0: %+v", tp.Hops[0].Probes[0])
	}

	// Hop 2 — two probes (the third was a timeout slot and
	// trimmed). Carries an [AS 64500] annotation in slot 1.
	if got := len(tp.Hops[1].Probes); got != 2 {
		t.Errorf("hop2: probes = %d, want 2 (trailing timeout trimmed)", got)
	}
	if tp.Hops[1].Probes[0].Asn != 64500 {
		t.Errorf("hop2 probe0 asn = %d, want 64500", tp.Hops[1].Probes[0].Asn)
	}
	// Hop 3 — pure star; one synthetic empty probe.
	if got := len(tp.Hops[2].Probes); got != 1 {
		t.Errorf("hop3: probes = %d, want 1 (synthetic empty)", got)
	}
	if tp.Hops[2].Probes[0].Ip != "" || tp.Hops[2].Probes[0].RttMs != 0 {
		t.Errorf("hop3 probe0 should be empty: %+v", tp.Hops[2].Probes[0])
	}
	// Hop 4 — three resolved probes for the destination.
	if got := len(tp.Hops[3].Probes); got != 3 {
		t.Errorf("hop4: probes = %d, want 3", got)
	}
	if tp.Hops[3].Probes[0].Ip != "1.1.1.1" ||
		tp.Hops[3].Probes[0].Hostname != "one.one.one.one" {
		t.Errorf("hop4 probe0: %+v", tp.Hops[3].Probes[0])
	}
}

const juniperPingSample = `PING 1.1.1.1 (1.1.1.1): 56 data bytes
!!!!!
--- 1.1.1.1 ping statistics ---
5 packets transmitted, 5 packets received, 0% packet loss
round-trip min/avg/max/stddev = 1.234/2.345/3.456/0.789 ms
`

func TestTextFSMParserJuniperPing(t *testing.T) {
	r := TextFSMParser{}.Parse(OpPing, []byte(juniperPingSample),
		Config{Template: "juniper_junos_ping"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	stats, ok := r.Payload.(*pb.PingStats)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if stats.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", stats.Target)
	}
	if stats.PacketsSent != 5 || stats.PacketsReceived != 5 {
		t.Errorf("packets sent=%d recv=%d, want 5/5",
			stats.PacketsSent, stats.PacketsReceived)
	}
	if stats.LossPct != 0 {
		t.Errorf("LossPct = %f, want 0", stats.LossPct)
	}
	if stats.RttAvgMs < 2.34 || stats.RttAvgMs > 2.35 {
		t.Errorf("RttAvgMs = %f, want ~2.345", stats.RttAvgMs)
	}
}

const juniperTracerouteSample = `traceroute to 1.1.1.1 (1.1.1.1), 30 hops max, 40 byte packets
 1  gw.example.com (10.0.0.254)  0.234 ms  0.222 ms  0.211 ms
 2  192.0.2.1  1.123 ms  1.234 ms  1.345 ms
 3  * * *
 4  one.one.one.one (1.1.1.1)  8.123 ms  8.234 ms  8.345 ms
`

func TestTextFSMParserJuniperTraceroute(t *testing.T) {
	r := TextFSMParser{}.Parse(OpTraceroute, []byte(juniperTracerouteSample),
		Config{Template: "juniper_junos_traceroute"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	tp, ok := r.Payload.(*pb.TracerouteParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if tp.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", tp.Target)
	}
	if got := len(tp.Hops); got != 4 {
		t.Fatalf("len(Hops) = %d, want 4", got)
	}
	if got := len(tp.Hops[0].Probes); got != 3 {
		t.Errorf("hop1: probes = %d, want 3", got)
	}
	if tp.Hops[0].Probes[0].Hostname != "gw.example.com" ||
		tp.Hops[0].Probes[0].Ip != "10.0.0.254" {
		t.Errorf("hop1 probe0: %+v", tp.Hops[0].Probes[0])
	}
	// Hop 3 — pure star, single synthetic empty probe.
	if got := len(tp.Hops[2].Probes); got != 1 {
		t.Errorf("hop3: probes = %d, want 1", got)
	}
	if tp.Hops[2].Probes[0].Ip != "" {
		t.Errorf("hop3 probe0 should be empty: %+v", tp.Hops[2].Probes[0])
	}
	// Hop 4 — destination with resolved hostname.
	if tp.Hops[3].Probes[0].Hostname != "one.one.one.one" {
		t.Errorf("hop4 probe0: %+v", tp.Hops[3].Probes[0])
	}
}

const nokiaPingSample = `PING 1.1.1.1 56 data bytes
64 bytes from 1.1.1.1: icmp_seq=1 ttl=58 time=2.04ms.
64 bytes from 1.1.1.1: icmp_seq=2 ttl=58 time=2.10ms.
64 bytes from 1.1.1.1: icmp_seq=3 ttl=58 time=2.06ms.
64 bytes from 1.1.1.1: icmp_seq=4 ttl=58 time=2.05ms.
64 bytes from 1.1.1.1: icmp_seq=5 ttl=58 time=2.07ms.

---- 1.1.1.1 PING Statistics ----
5 packets transmitted, 5 packets received, 0.00% packet loss
round-trip min = 1.987ms, avg = 2.123ms, max = 2.456ms, stddev = 0.156ms
`

func TestTextFSMParserNokiaPing(t *testing.T) {
	r := TextFSMParser{}.Parse(OpPing, []byte(nokiaPingSample),
		Config{Template: "nokia_sros_ping"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	stats, ok := r.Payload.(*pb.PingStats)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if stats.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", stats.Target)
	}
	if stats.PacketsSent != 5 || stats.PacketsReceived != 5 {
		t.Errorf("packets sent=%d recv=%d", stats.PacketsSent, stats.PacketsReceived)
	}
	if stats.RttMinMs < 1.98 || stats.RttMinMs > 1.99 {
		t.Errorf("RttMinMs = %f, want ~1.987", stats.RttMinMs)
	}
	if stats.RttMdevMs < 0.15 || stats.RttMdevMs > 0.16 {
		t.Errorf("RttMdevMs = %f, want ~0.156", stats.RttMdevMs)
	}
}

const nokiaTracerouteSample = `traceroute to 1.1.1.1, 30 hops max, 40 byte packets
 1  10.0.0.254 (10.0.0.254)  0.234 ms  0.222 ms  0.211 ms
 2  one.one.one.one (1.1.1.1)  8 ms  8 ms  8 ms
 3  * * *
`

func TestTextFSMParserNokiaTraceroute(t *testing.T) {
	r := TextFSMParser{}.Parse(OpTraceroute, []byte(nokiaTracerouteSample),
		Config{Template: "nokia_sros_traceroute"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	tp, ok := r.Payload.(*pb.TracerouteParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if tp.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", tp.Target)
	}
	if got := len(tp.Hops); got != 3 {
		t.Fatalf("len(Hops) = %d, want 3", got)
	}
	if got := len(tp.Hops[0].Probes); got != 3 {
		t.Errorf("hop1: probes = %d, want 3", got)
	}
	if tp.Hops[1].Probes[0].Hostname != "one.one.one.one" {
		t.Errorf("hop2 probe0 hostname: %+v", tp.Hops[1].Probes[0])
	}
	if got := len(tp.Hops[2].Probes); got != 1 {
		t.Errorf("hop3 (star): probes = %d, want 1", got)
	}
}

const mikrotikPingSample = `  SEQ HOST                                     SIZE TTL TIME       STATUS
    0 1.1.1.1                                    56  58 2ms        echo reply
    1 1.1.1.1                                    56  58 2ms        echo reply
    2 1.1.1.1                                    56  58 2ms        echo reply
    3 1.1.1.1                                    56  58 2ms        echo reply
    4 1.1.1.1                                    56  58 2ms        echo reply
    sent=5 received=5 packet-loss=0% min-rtt=1ms avg-rtt=2ms max-rtt=4ms
`

func TestTextFSMParserMikroTikPing(t *testing.T) {
	r := TextFSMParser{}.Parse(OpPing, []byte(mikrotikPingSample),
		Config{Template: "mikrotik_routeros_ping"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	stats, ok := r.Payload.(*pb.PingStats)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if stats.Target != "1.1.1.1" {
		t.Errorf("Target = %q, want 1.1.1.1", stats.Target)
	}
	if stats.PacketsSent != 5 || stats.PacketsReceived != 5 {
		t.Errorf("packets sent=%d recv=%d", stats.PacketsSent, stats.PacketsReceived)
	}
	if stats.RttMinMs != 1 || stats.RttAvgMs != 2 || stats.RttMaxMs != 4 {
		t.Errorf("rtt: min=%f avg=%f max=%f",
			stats.RttMinMs, stats.RttAvgMs, stats.RttMaxMs)
	}
}

const mikrotikTracerouteSample = ` # ADDRESS                                    LOSS SENT    LAST     AVG    BEST   WORST STD-DEV STATUS
 1 10.0.0.254                                   0%    1   0.2ms   0.2ms   0.2ms   0.2ms       0
 2 192.0.2.1                                    0%    1   1.1ms   1.1ms   1.1ms   1.1ms       0
 3 (timeout)                                  100%    1                                       0
 4 1.1.1.1                                      0%    1   8.0ms   8.0ms   8.0ms   8.0ms       0
`

func TestTextFSMParserMikroTikTraceroute(t *testing.T) {
	r := TextFSMParser{}.Parse(OpTraceroute, []byte(mikrotikTracerouteSample),
		Config{Template: "mikrotik_routeros_traceroute"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	tp, ok := r.Payload.(*pb.TracerouteParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(tp.Hops); got != 4 {
		t.Fatalf("len(Hops) = %d, want 4", got)
	}
	// Each hop has one probe (RouterOS single-AVG convention).
	if got := len(tp.Hops[0].Probes); got != 1 {
		t.Errorf("hop1: probes = %d, want 1", got)
	}
	if tp.Hops[0].Probes[0].Ip != "10.0.0.254" {
		t.Errorf("hop1 probe0 ip: %+v", tp.Hops[0].Probes[0])
	}
	// Hop 3 is a timeout — empty probe.
	if tp.Hops[2].Probes[0].Ip != "" || tp.Hops[2].Probes[0].RttMs != 0 {
		t.Errorf("hop3 (timeout) should be empty: %+v", tp.Hops[2].Probes[0])
	}
	// Hop 4 — destination probe carries the destination IP.
	if tp.Hops[3].Probes[0].Ip != "1.1.1.1" {
		t.Errorf("hop4 probe0 ip: %+v", tp.Hops[3].Probes[0])
	}
}

const aristaBGPPathsSample = `BGP routing table entry for 1.1.1.0/24
 Paths: (2 available, best #1)
 Advertised to peers:
  65001 65002
    fe80::1 from fe80::1 (10.0.0.2)
      Origin IGP, metric 0, localpref 100, weight 0, valid, external, best
      Community: 65000:100 65000:200
      Large Community: 214503:8:3607
      Last update: 1d02h ago
`

func TestTextFSMParserAristaBGPPaths(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPRoute, []byte(aristaBGPPathsSample),
		Config{Template: "arista_eos_show_bgp_paths"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(paths.Paths); got != 1 {
		t.Fatalf("len(Paths) = %d, want 1", got)
	}
	p := paths.Paths[0]
	if p.Prefix != "1.1.1.0/24" {
		t.Errorf("Prefix = %q", p.Prefix)
	}
	if got, want := p.AsPath, []uint32{65001, 65002}; !equalU32(got, want) {
		t.Errorf("AsPath = %v, want %v", got, want)
	}
	if p.Origin != "igp" {
		t.Errorf("Origin = %q, want igp", p.Origin)
	}
	if p.Med != 0 || p.LocalPref != 100 {
		t.Errorf("Med=%d LocalPref=%d, want 0/100", p.Med, p.LocalPref)
	}
	if !p.Best {
		t.Errorf("Best=false, want true")
	}
	if got, want := p.Communities, []string{"65000:100", "65000:200"}; !equalStr(got, want) {
		t.Errorf("Communities = %v, want %v", got, want)
	}
	if got, want := p.LargeCommunities, []string{"214503:8:3607"}; !equalStr(got, want) {
		t.Errorf("LargeCommunities = %v, want %v", got, want)
	}
	if p.AgeSeconds == 0 {
		t.Errorf("AgeSeconds = 0, want non-zero (1d02h ≈ 93600s)")
	}
}

const aristaBGPPathsModernSample = `BGP routing table entry for 43.157.192.0/18
 Paths: 1 available
  3399 5511 7713 132203
    195.128.254.232 from 195.128.254.232 (195.128.255.252)
      Origin IGP, metric 0, localpref 100, IGP metric 0, weight 0, tag 0
      Received 2d19h ago, valid, external, best
      Community: 1:2 62:2 852:62
      Extended Community: Link-Bandwidth-AS:3399:12.5 Bps
      Large Community: 214503:1:2 214503:2:2
      Rx SAFI: Unicast
`

func TestTextFSMParserAristaBGPPathsModern(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPRoute, []byte(aristaBGPPathsModernSample),
		Config{Template: "arista_eos_show_bgp_paths"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(paths.Paths); got != 1 {
		t.Fatalf("len(Paths) = %d, want 1", got)
	}
	p := paths.Paths[0]
	if p.Prefix != "43.157.192.0/18" {
		t.Errorf("Prefix = %q", p.Prefix)
	}
	if got, want := p.AsPath, []uint32{3399, 5511, 7713, 132203}; !equalU32(got, want) {
		t.Errorf("AsPath = %v, want %v", got, want)
	}
	if p.Origin != "igp" {
		t.Errorf("Origin = %q, want igp", p.Origin)
	}
	if p.Med != 0 || p.LocalPref != 100 {
		t.Errorf("Med=%d LocalPref=%d, want 0/100", p.Med, p.LocalPref)
	}
	if !p.Best {
		t.Errorf("Best=false, want true (captured from `Received … ago, …, best`)")
	}
	if got, want := p.Communities, []string{"1:2", "62:2", "852:62"}; !equalStr(got, want) {
		t.Errorf("Communities = %v, want %v", got, want)
	}
	if got, want := p.LargeCommunities, []string{"214503:1:2", "214503:2:2"}; !equalStr(got, want) {
		t.Errorf("LargeCommunities = %v, want %v", got, want)
	}
	if p.AgeSeconds == 0 {
		t.Errorf("AgeSeconds = 0, want non-zero (2d19h ≈ 240000s)")
	}
}

const aristaBGPPathsAggregatedSample = `BGP routing table information for VRF PROUD-BEAVER
Router identifier 0.0.0.4, local AS number 214503
BGP routing table entry for 1.1.1.0/24
 Paths: 2 available
  13335 (aggregated by 13335 10.34.48.128)
    185.1.215.60 from 185.1.215.60 (172.68.180.34)
      Origin IGP, metric 0, localpref 200, IGP metric 0, weight 0, tag 0
      Received 22d08h ago, valid, external, best
      Community: 13335:10128
      Extended Community: Link-Bandwidth-AS:13335:50.0 Bps
      Large Community: 214503:1:3 214503:2:2 214503:3:14 214503:5:13335 214503:7:2 214503:8:3607
      Rx SAFI: Unicast
  3399 13335 (aggregated by 13335 10.34.48.128)
    195.128.254.232 from 195.128.254.232 (195.128.255.252)
      Origin IGP, metric 0, localpref 100, IGP metric 0, weight 0, tag 0
      Received 3d20h ago, valid, external
      Community: 3399:200 3399:201 3399:500 3399:600 3399:702 3399:2027 13335:10128
      Extended Community: Link-Bandwidth-AS:3399:12.5 Bps
      Large Community: 214503:1:2 214503:2:2 214503:3:14 214503:5:3399 214503:7:2
      Rx SAFI: Unicast
`

func TestTextFSMParserAristaBGPPathsAggregated(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPRoute, []byte(aristaBGPPathsAggregatedSample),
		Config{Template: "arista_eos_show_bgp_paths"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T, want *pb.BGPPaths", r.Payload)
	}
	if got := len(paths.Paths); got != 2 {
		t.Fatalf("len(Paths) = %d, want 2", got)
	}
	// ---- Path 0: best, single-ASN AS-path from the aggregator ----
	p0 := paths.Paths[0]
	if p0.Prefix != "1.1.1.0/24" {
		t.Errorf("path0 Prefix = %q, want 1.1.1.0/24", p0.Prefix)
	}
	// THE REGRESSION ASSERTION — AS-path must not be empty even
	// though the bare-ASN line carried an `(aggregated by …)`
	// trailing annotation.
	if got, want := p0.AsPath, []uint32{13335}; !equalU32(got, want) {
		t.Errorf("path0 AsPath = %v, want %v", got, want)
	}
	if p0.Origin != "igp" {
		t.Errorf("path0 Origin = %q, want igp", p0.Origin)
	}
	if p0.LocalPref != 200 {
		t.Errorf("path0 LocalPref = %d, want 200", p0.LocalPref)
	}
	if !p0.Best {
		t.Errorf("path0 Best = false, want true (`Received … ago, …, best`)")
	}
	if p0.Nexthop != "185.1.215.60" {
		t.Errorf("path0 Nexthop = %q, want 185.1.215.60", p0.Nexthop)
	}
	if got, want := p0.Communities, []string{"13335:10128"}; !equalStr(got, want) {
		t.Errorf("path0 Communities = %v, want %v", got, want)
	}
	if got, want := p0.LargeCommunities, []string{
		"214503:1:3", "214503:2:2", "214503:3:14",
		"214503:5:13335", "214503:7:2", "214503:8:3607",
	}; !equalStr(got, want) {
		t.Errorf("path0 LargeCommunities = %v, want %v", got, want)
	}
	if p0.AgeSeconds == 0 {
		t.Errorf("path0 AgeSeconds = 0, want non-zero (22d08h ≈ 1929600s)")
	}
	// ---- Path 1: non-best, two-ASN AS-path through transit ----
	p1 := paths.Paths[1]
	if p1.Prefix != "1.1.1.0/24" {
		t.Errorf("path1 Prefix = %q, want 1.1.1.0/24 (Filldown)", p1.Prefix)
	}
	// Second regression assertion — multi-ASN AS-path also survives
	// the aggregator-annotation suffix.
	if got, want := p1.AsPath, []uint32{3399, 13335}; !equalU32(got, want) {
		t.Errorf("path1 AsPath = %v, want %v", got, want)
	}
	if p1.LocalPref != 100 {
		t.Errorf("path1 LocalPref = %d, want 100", p1.LocalPref)
	}
	if p1.Best {
		t.Errorf("path1 Best = true, want false (Received line lacked `best`)")
	}
	if p1.Nexthop != "195.128.254.232" {
		t.Errorf("path1 Nexthop = %q, want 195.128.254.232", p1.Nexthop)
	}
	if got, want := p1.Communities, []string{
		"3399:200", "3399:201", "3399:500", "3399:600",
		"3399:702", "3399:2027", "13335:10128",
	}; !equalStr(got, want) {
		t.Errorf("path1 Communities = %v, want %v", got, want)
	}
}

const aristaBGPTableSample = `BGP routing table information for VRF PROUD-BEAVER
Router identifier 0.0.0.4, local AS number 214503
Route status codes: s - suppressed, * - valid, > - active, E - ECMP head, e - ECMP
                    S - Stale, c - Contributing to ECMP, b - backup, L - labeled-unicast
                    % - Pending BGP convergence
Origin codes: i - IGP, e - EGP, ? - incomplete
RPKI Origin Validation codes: V - valid, I - invalid, U - unknown
AS Path Attributes: Or-ID - Originator ID, C-LST - Cluster List, LL Nexthop - Link Local Nexthop

          Network                Next Hop              Metric  AIGP       LocPref Weight  Path
 * >      45.141.108.0/22        195.128.254.232       0       -          100     0       3399 i
 * >      93.164.128.0/19        195.128.254.232       0       -          100     0       3399 i
 * >      178.132.72.0/21        195.128.254.232       0       -          100     0       3399 i
 * >      185.86.104.0/22        195.128.254.232       0       -          100     0       3399 i
 * >      185.147.236.0/22       195.128.254.232       0       -          100     0       3399 i
 * >      185.242.228.0/22       195.128.254.232       0       -          100     0       3399 i
 * >      192.36.22.0/24         195.128.254.232       0       -          100     0       3399 i
 * >      192.36.208.0/24        195.128.254.232       0       -          100     0       3399 i
 * >      192.165.178.0/23       195.128.254.232       0       -          100     0       3399 i
 * >      193.178.129.0/24       195.128.254.232       0       -          100     0       3399 i
 * >      193.180.23.0/24        195.128.254.232       0       -          100     0       3399 i
 * >      193.180.96.0/22        195.128.254.232       0       -          100     0       3399 i
 * >      193.180.164.0/23       195.128.254.232       0       -          100     0       3399 i
 * >      193.181.248.0/22       195.128.254.232       0       -          100     0       3399 i
 * >      193.183.68.0/23        195.128.254.232       0       -          100     0       3399 i
 * >      193.183.116.0/24       195.128.254.232       0       -          100     0       3399 i
 * >      193.183.132.0/23       195.128.254.232       0       -          100     0       3399 i
 * >      194.14.188.0/23        195.128.254.232       0       -          100     0       3399 i
 * >      194.32.144.0/23        195.128.254.232       0       -          100     0       3399 i
 * >      194.68.170.0/23        195.128.254.232       0       -          100     0       3399 i
 * >      194.68.220.0/23        195.128.254.232       0       -          100     0       3399 i
 * >      194.71.216.0/23        195.128.254.232       0       -          100     0       3399 i
 * >      194.103.80.0/22        195.128.254.232       0       -          100     0       3399 i
 * >      194.132.40.0/22        195.128.254.232       0       -          100     0       3399 i
 * >      195.128.240.0/23       195.128.254.232       0       -          100     0       3399 i
 * >      195.128.254.0/23       195.128.254.232       0       -          100     0       3399 i
`

// TestTextFSMParserAristaBGPTable verifies parsing of the EOS RIB
// table form emitted by `show ip bgp regexp`.
func TestTextFSMParserAristaBGPTable(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPASPath, []byte(aristaBGPTableSample),
		Config{Template: "arista_eos_show_bgp_table"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T, want *pb.BGPPaths", r.Payload)
	}
	// Regression assertion — the bug today produces zero paths.
	if got := len(paths.Paths); got != 26 {
		t.Fatalf("len(Paths) = %d, want 26 (real rows; "+
			"trailing Filldown residue should be filtered)", got)
	}
	// ---- Path 0: first row, fully populated ----
	p0 := paths.Paths[0]
	if p0.Prefix != "45.141.108.0/22" {
		t.Errorf("path0 Prefix = %q, want 45.141.108.0/22", p0.Prefix)
	}
	if p0.Nexthop != "195.128.254.232" {
		t.Errorf("path0 Nexthop = %q, want 195.128.254.232", p0.Nexthop)
	}
	if got, want := p0.AsPath, []uint32{3399}; !equalU32(got, want) {
		t.Errorf("path0 AsPath = %v, want %v", got, want)
	}
	if !p0.Best {
		t.Errorf("path0 Best = false, want true (`>` glyph present)")
	}
	if p0.LocalPref != 100 {
		t.Errorf("path0 LocalPref = %d, want 100", p0.LocalPref)
	}
	if p0.Med != 0 {
		t.Errorf("path0 Med = %d, want 0", p0.Med)
	}
	// `i` lower-cased by tfsmBGPPathsResult's strings.ToLower
	// call; stays as the single-letter EOS origin code (no IANA
	// expansion in scope today).
	if p0.Origin != "i" {
		t.Errorf("path0 Origin = %q, want %q", p0.Origin, "i")
	}
	// ---- Spot-check a longer-prefix row mid-table ----
	var p13 *pb.BGPPath
	for _, p := range paths.Paths {
		if p.Prefix == "193.180.164.0/23" {
			p13 = p
			break
		}
	}
	if p13 == nil {
		t.Fatalf("expected to find 193.180.164.0/23 row in parsed paths")
	}
	if got, want := p13.AsPath, []uint32{3399}; !equalU32(got, want) {
		t.Errorf("193.180.164.0/23 AsPath = %v, want %v", got, want)
	}
	if !p13.Best {
		t.Errorf("193.180.164.0/23 Best = false, want true")
	}
	// ---- Last row: alignment-drift canary ----
	last := paths.Paths[len(paths.Paths)-1]
	if last.Prefix != "195.128.254.0/23" {
		t.Errorf("last Prefix = %q, want 195.128.254.0/23", last.Prefix)
	}
	if got, want := last.AsPath, []uint32{3399}; !equalU32(got, want) {
		t.Errorf("last AsPath = %v, want %v", got, want)
	}
}

const ciscoBGPPathsSample = `BGP routing table entry for 1.1.1.0/24, version 5
  Paths: (1 available, best #1, table default)
  Advertised to update-groups:
     1
  Refresh Epoch 1
  65001 65002
    10.0.0.2 from 10.0.0.2 (10.0.0.2)
      Origin IGP, metric 0, localpref 100, valid, external, best
      Community: 65000:100 65000:200
      Large-Community: 214503:8:3607
      rx pathid: 0, tx pathid: 0x0
      Updated on Aug 25 2025 16:00:00 UTC
`

func TestTextFSMParserCiscoIOSBGPPaths(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPRoute, []byte(ciscoBGPPathsSample),
		Config{Template: "cisco_ios_show_bgp_paths"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(paths.Paths); got != 1 {
		t.Fatalf("len(Paths) = %d, want 1", got)
	}
	p := paths.Paths[0]
	if p.Prefix != "1.1.1.0/24" {
		t.Errorf("Prefix = %q", p.Prefix)
	}
	if got, want := p.AsPath, []uint32{65001, 65002}; !equalU32(got, want) {
		t.Errorf("AsPath = %v, want %v", got, want)
	}
	if p.Origin != "igp" {
		t.Errorf("Origin = %q, want igp", p.Origin)
	}
	if !p.Best {
		t.Errorf("Best=false, want true")
	}
	if got, want := p.Communities, []string{"65000:100", "65000:200"}; !equalStr(got, want) {
		t.Errorf("Communities = %v, want %v", got, want)
	}
	if got, want := p.LargeCommunities, []string{"214503:8:3607"}; !equalStr(got, want) {
		t.Errorf("LargeCommunities = %v, want %v", got, want)
	}
}

const juniperBGPPathsSample = `inet.0: 1000 destinations, 2000 routes (1000 active, 0 holddown, 0 hidden)
1.1.1.0/24 (2 entries, 1 announced)
        *BGP    Preference: 170/-101
                Next hop type: Router, Next hop index: 1234
                Source: 10.0.0.2
                Next hop: 10.0.0.2 via xe-0/0/0.0, selected
                State: <Active Ext>
                Local AS: 65000 Peer AS: 65001
                Age: 1d 2:00:00     Metric: 0
                AS path: 65001 65002 I
                Communities: 65000:100 65000:200
                Large Communities: 214503:8:3607
                Accepted
                Localpref: 100
                Router ID: 10.0.0.2
`

func TestTextFSMParserJuniperBGPPaths(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPRoute, []byte(juniperBGPPathsSample),
		Config{Template: "juniper_junos_show_route_bgp"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(paths.Paths); got != 1 {
		t.Fatalf("len(Paths) = %d, want 1", got)
	}
	p := paths.Paths[0]
	if p.Prefix != "1.1.1.0/24" {
		t.Errorf("Prefix = %q", p.Prefix)
	}
	if got, want := p.AsPath, []uint32{65001, 65002}; !equalU32(got, want) {
		t.Errorf("AsPath = %v, want %v", got, want)
	}
	// Origin in JunOS: I/E/?. Lowercased on the wire.
	if p.Origin != "i" {
		t.Errorf("Origin = %q, want i", p.Origin)
	}
	if !p.Best {
		t.Errorf("Best=false, want true (`*BGP` marker)")
	}
	if p.PeerAsn != 65001 {
		t.Errorf("PeerAsn = %d, want 65001", p.PeerAsn)
	}
	if p.LocalPref != 100 {
		t.Errorf("LocalPref = %d, want 100", p.LocalPref)
	}
	if got, want := p.Communities, []string{"65000:100", "65000:200"}; !equalStr(got, want) {
		t.Errorf("Communities = %v, want %v", got, want)
	}
}

const nokiaBGPRoutesSample = `===============================================================================
BGP Router ID:10.0.0.1        AS:65000       Local AS:65000
BGP RIB-In Entries
-------------------------------------------------------------------------------
Network        : 1.1.1.0/24
Nexthop        : 10.0.0.2
From           : 10.0.0.2
Res. Nexthop   : 10.0.0.2
Local Pref.    : 100                    Interface Name : to-peer
Aggregator AS  : None                   Aggregator     : None
Atomic Aggr.   : Not Atomic             MED            : 0
AIGP Metric    : None
Connector      :
Community      : 65000:100 65000:200
Large Community: 214503:8:3607
Cluster        : No Cluster Members
Originator Id  : None                   Peer Router Id : 10.0.0.2
IPv4 Label     : N/A
Flags          : Used Valid Best IGP
Route Source   : External
AS-Path        : 65001 65002
Peer AS        : 65001
Age            : 1d02h00m00s
-------------------------------------------------------------------------------
`

func TestTextFSMParserNokiaBGPRoutes(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPRoute, []byte(nokiaBGPRoutesSample),
		Config{Template: "nokia_sros_show_bgp_routes"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(paths.Paths); got != 1 {
		t.Fatalf("len(Paths) = %d, want 1", got)
	}
	p := paths.Paths[0]
	if p.Prefix != "1.1.1.0/24" {
		t.Errorf("Prefix = %q", p.Prefix)
	}
	if got, want := p.AsPath, []uint32{65001, 65002}; !equalU32(got, want) {
		t.Errorf("AsPath = %v, want %v", got, want)
	}
	if !p.Best {
		t.Errorf("Best=false, want true (Flags carries `Best`)")
	}
	if p.LocalPref != 100 {
		t.Errorf("LocalPref = %d, want 100", p.LocalPref)
	}
	if p.PeerAsn != 65001 {
		t.Errorf("PeerAsn = %d, want 65001", p.PeerAsn)
	}
	if got, want := p.Communities, []string{"65000:100", "65000:200"}; !equalStr(got, want) {
		t.Errorf("Communities = %v, want %v", got, want)
	}
	if p.AgeSeconds == 0 {
		t.Errorf("AgeSeconds = 0, want non-zero (1d02h00m00s ≈ 93600s)")
	}
}

const mikrotikBGPAdvSample = ` Flags: A - active
  0 A peer="peer1" dst-address=1.1.1.0/24
      nexthop=10.0.0.2 origin=igp local-pref=100 med=0
      as-path="65001,65002"
      communities=65000:100,65000:200
      large-communities=214503:8:3607
`

func TestTextFSMParserMikroTikBGPAdvertisements(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPRoute, []byte(mikrotikBGPAdvSample),
		Config{Template: "mikrotik_routeros_show_bgp_advertisements"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(paths.Paths); got != 1 {
		t.Fatalf("len(Paths) = %d, want 1", got)
	}
	p := paths.Paths[0]
	if p.Prefix != "1.1.1.0/24" {
		t.Errorf("Prefix = %q", p.Prefix)
	}
	// AS-path is comma-separated in RouterOS; projector splits.
	if got, want := p.AsPath, []uint32{65001, 65002}; !equalU32(got, want) {
		t.Errorf("AsPath = %v, want %v", got, want)
	}
	if p.Origin != "igp" {
		t.Errorf("Origin = %q, want igp", p.Origin)
	}
	if !p.Best {
		t.Errorf("Best=false, want true (`A` flag)")
	}
	if got, want := p.Communities, []string{"65000:100", "65000:200"}; !equalStr(got, want) {
		t.Errorf("Communities = %v, want %v", got, want)
	}
}

const aristaBGPSummarySample = `BGP summary information for VRF default
Router identifier 10.0.0.1, local AS number 65000
Neighbor        V AS           MsgRcvd MsgSent  InQ OutQ  Up/Down State  PfxRcd PfxAcc
  10.0.0.2      4 65001            123     124    0    0 1d02h  Estab   100    100
  10.0.0.3      4 65002              0       0    0    0 never  Idle      0      0
  fe80::1       4 65003               5      10    0    0 00:01:00 OpenSent
`

func TestTextFSMParserAristaBGPSummary(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPSummary, []byte(aristaBGPSummarySample),
		Config{Template: "arista_eos_show_bgp_summary"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	s, ok := r.Payload.(*pb.BGPSummaryParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if s.LocalAsn != 65000 || s.RouterId != "10.0.0.1" {
		t.Errorf("local_asn=%d router_id=%q", s.LocalAsn, s.RouterId)
	}
	if got := len(s.Peers); got != 3 {
		t.Fatalf("len(Peers) = %d, want 3", got)
	}
	// Peer 1: Established → "estab" → projector → "established".
	if s.Peers[0].State != "established" {
		t.Errorf("peer0 state = %q, want established", s.Peers[0].State)
	}
	if s.Peers[0].PrefixesReceived != 100 {
		t.Errorf("peer0 pfxrecv = %d, want 100", s.Peers[0].PrefixesReceived)
	}
	// Peer 2: Idle stays lowercased.
	if s.Peers[1].State != "idle" {
		t.Errorf("peer1 state = %q, want idle", s.Peers[1].State)
	}
	// Peer 3: OpenSent stays lowercased.
	if s.Peers[2].State != "opensent" {
		t.Errorf("peer2 state = %q, want opensent", s.Peers[2].State)
	}
	// Legacy shape has no Description column; the optional
	// description-capture group on the row pattern must not have
	// accidentally swallowed leading row indent. All three peers
	// should have empty descriptions.
	for i, p := range s.Peers {
		if p.Description != "" {
			t.Errorf("legacy peer%d Description = %q, want empty", i, p.Description)
		}
	}
}

const aristaBGPSummaryModernSample = `BGP summary information for VRF PROUD-BEAVER
Router identifier 0.0.0.4, local AS number 214503
Neighbor Status Codes: m - Under maintenance
  Description              Neighbor         V AS           MsgRcvd   MsgSent  InQ OutQ  Up/Down State   PfxRcd PfxAcc
  SONIX Route Servers      185.1.215.1      4 61229        3631089    120410    0    0   22d08h Estab   102543 102543
  no ack hosting network   185.1.215.48     4 30893              0     17373    0    0   35d21h OpenSent
  STHIX Route Servers      192.121.80.1     4 57151              0         0    0    0   35d21h Idle(NoIf)

BGP summary information for VRF PROUD-BEAVER
Router identifier 0.0.0.4, local AS number 214503
Neighbor Status Codes: m - Under maintenance
  Description              Neighbor         V AS           MsgRcvd   MsgSent  InQ OutQ  Up/Down State   PfxRcd PfxAcc
  NORDUnet                 2001:7f8:117::2603:1 4 2603           93112    104145    0    0   22d08h Estab   279    279
  Hurricane Electric       2001:7f8:117::6939:1 4 6939         7795704    120136    0    0   16d08h Estab   235779 235779
  IBGP-rt2                 fd00:3:191e:1:2::1 4 203038             0         0    0    0   35d21h Idle(NoIf)
`

func TestTextFSMParserAristaBGPSummaryModern(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPSummary, []byte(aristaBGPSummaryModernSample),
		Config{Template: "arista_eos_show_bgp_summary"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	s, ok := r.Payload.(*pb.BGPSummaryParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if s.LocalAsn != 214503 || s.RouterId != "0.0.0.4" {
		t.Errorf("local_asn=%d router_id=%q", s.LocalAsn, s.RouterId)
	}
	// Expect six peers total across the two back-to-back VRF blocks.
	if got := len(s.Peers); got != 6 {
		t.Fatalf("len(Peers) = %d, want 6", got)
	}
	// v4 block — peer 0 is Established with description containing spaces.
	if s.Peers[0].PeerIp != "185.1.215.1" {
		t.Errorf("peer0 ip = %q, want 185.1.215.1", s.Peers[0].PeerIp)
	}
	if s.Peers[0].State != "established" {
		t.Errorf("peer0 state = %q, want established", s.Peers[0].State)
	}
	if s.Peers[0].PrefixesReceived != 102543 || s.Peers[0].PrefixesAccepted != 102543 {
		t.Errorf("peer0 pfx recv/acc = %d/%d, want 102543/102543",
			s.Peers[0].PrefixesReceived, s.Peers[0].PrefixesAccepted)
	}
	// Peer 1: OpenSent (non-Established, no pfx counts).
	if s.Peers[1].State != "opensent" {
		t.Errorf("peer1 state = %q, want opensent", s.Peers[1].State)
	}
	// Peer 2: Idle(NoIf) — inline-paren qualifier preserved by the
	// projector's lowercasing pass.
	if s.Peers[2].State != "idle(noif)" {
		t.Errorf("peer2 state = %q, want idle(noif)", s.Peers[2].State)
	}
	// v6 block — peer 3 is the first v6 peer.
	if s.Peers[3].PeerIp != "2001:7f8:117::2603:1" {
		t.Errorf("peer3 ip = %q, want 2001:7f8:117::2603:1", s.Peers[3].PeerIp)
	}
	if s.Peers[3].State != "established" {
		t.Errorf("peer3 state = %q, want established", s.Peers[3].State)
	}
	if s.Peers[3].PrefixesReceived != 279 {
		t.Errorf("peer3 pfxrecv = %d, want 279", s.Peers[3].PrefixesReceived)
	}
	// Peer 5: v6 Idle(NoIf).
	if s.Peers[5].State != "idle(noif)" {
		t.Errorf("peer5 state = %q, want idle(noif)", s.Peers[5].State)
	}
	// Description column — the live-box paste exercises every
	// shape we care about:
	//   * Multi-token, single internal spaces (`SONIX Route Servers`).
	//   * Multi-token, multiple internal spaces ("no ack hosting
	//     network") — the lazy-match canary; if the regex switched
	//     to greedy or anchored on `\s+` the trailing space would
	//     bleed into peer_ip.
	//   * Single-token survives the v4→v6 Start re-transition
	//     (`NORDUnet`, `IBGP-rt2`).
	wantDesc := []string{
		"SONIX Route Servers",
		"no ack hosting network",
		"STHIX Route Servers",
		"NORDUnet",
		"Hurricane Electric",
		"IBGP-rt2",
	}
	for i, want := range wantDesc {
		if s.Peers[i].Description != want {
			t.Errorf("peer%d Description = %q, want %q",
				i, s.Peers[i].Description, want)
		}
	}
}

const ciscoBGPSummarySample = `BGP router identifier 10.0.0.1, local AS number 65000
BGP table version is 100, main routing table version 100
100 network entries using 12345 bytes of memory

Neighbor        V           AS MsgRcvd MsgSent   TblVer  InQ OutQ Up/Down  State/PfxRcd
10.0.0.2        4        65001     123     124      100    0    0 1d02h        100
10.0.0.3        4        65002       0       0        1    0    0 never        Idle
2001:db8::2     4        65003     234     235      100    0    0 00:01:00 OpenSent
`

func TestTextFSMParserCiscoIOSBGPSummary(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPSummary, []byte(ciscoBGPSummarySample),
		Config{Template: "cisco_ios_show_bgp_summary"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	s, ok := r.Payload.(*pb.BGPSummaryParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if s.LocalAsn != 65000 {
		t.Errorf("local_asn = %d", s.LocalAsn)
	}
	if got := len(s.Peers); got != 3 {
		t.Fatalf("len(Peers) = %d, want 3", got)
	}
	// Peer 1: trailing "100" was captured into `state` slot; the
	// projector recognises it as numeric, moves it to
	// prefixes_received, and synthesises state="established".
	if s.Peers[0].State != "established" {
		t.Errorf("peer0 state = %q, want established", s.Peers[0].State)
	}
	if s.Peers[0].PrefixesReceived != 100 {
		t.Errorf("peer0 pfxrecv = %d, want 100", s.Peers[0].PrefixesReceived)
	}
	// Peer 2 & 3: non-Established keywords lowercased.
	if s.Peers[1].State != "idle" {
		t.Errorf("peer1 state = %q, want idle", s.Peers[1].State)
	}
	if s.Peers[2].State != "opensent" {
		t.Errorf("peer2 state = %q, want opensent", s.Peers[2].State)
	}
}

const juniperBGPSummarySample = `Threading mode: BGP I/O
Groups: 2 Peers: 3 Down peers: 0
Table          Tot Paths  Act Paths Suppressed    History Damp State    Pending
inet.0
                   1000        500          0          0          0          0
Peer                     AS      InPkt     OutPkt    OutQ   Flaps Last Up/Dwn State|#Active/Received/Accepted/Damped...
10.0.0.2              65001       1234       1235       0       0     1d2h Establ
  inet.0: 500/1000/500/0
10.0.0.3              65002          0          0       0       0    never Active
2001:db8::2           65003        234        235       0       0  0:01:00 Establ
  inet6.0: 100/200/100/0
`

func TestTextFSMParserJuniperBGPSummary(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPSummary, []byte(juniperBGPSummarySample),
		Config{Template: "juniper_junos_show_bgp_summary"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	s, ok := r.Payload.(*pb.BGPSummaryParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(s.Peers); got != 3 {
		t.Fatalf("len(Peers) = %d, want 3", got)
	}
	// Peer 1: Established → prefix counts captured from inet.0 line.
	if s.Peers[0].State != "established" {
		t.Errorf("peer0 state = %q, want established", s.Peers[0].State)
	}
	if s.Peers[0].PrefixesAccepted != 500 || s.Peers[0].PrefixesReceived != 1000 {
		t.Errorf("peer0 pfxacc=%d pfxrecv=%d",
			s.Peers[0].PrefixesAccepted, s.Peers[0].PrefixesReceived)
	}
	if s.Peers[0].AddressFamily != "inet.0" {
		t.Errorf("peer0 afi = %q", s.Peers[0].AddressFamily)
	}
	// Peer 2: Active (non-Established) — Record fires on the row line.
	if s.Peers[1].State != "active" {
		t.Errorf("peer1 state = %q, want active", s.Peers[1].State)
	}
	if s.Peers[1].PeerAsn != 65002 {
		t.Errorf("peer1 asn = %d", s.Peers[1].PeerAsn)
	}
	// Peer 3: Established v6.
	if s.Peers[2].State != "established" {
		t.Errorf("peer2 state = %q, want established", s.Peers[2].State)
	}
	if s.Peers[2].AddressFamily != "inet6.0" {
		t.Errorf("peer2 afi = %q", s.Peers[2].AddressFamily)
	}
}

const nokiaBGPSummarySample = `===============================================================================
BGP Summary
Legend : D - Dynamic Neighbor
Neighbor
Description
               AS PktRcvd InQ  Up/Down   State|Rcv/Act/Sent (Addr Family)
                  PktSent OutQ
-------------------------------------------------------------------------------
10.0.0.2
             65001    1234   0 1d02h00m  500/450/600 (IPv4)
                      1235   0
10.0.0.3
             65002       0   0 00h00m00s Active
                          0   0
2001:db8::2
             65003     234   0 00h01m00s 100/95/200 (IPv6)
                       235   0
`

func TestTextFSMParserNokiaBGPSummary(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPSummary, []byte(nokiaBGPSummarySample),
		Config{Template: "nokia_sros_show_bgp_summary"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	s, ok := r.Payload.(*pb.BGPSummaryParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(s.Peers); got != 3 {
		t.Fatalf("len(Peers) = %d, want 3", got)
	}
	// Peer 1: v4 Established with Rcv/Act/Sent triplet captured.
	if s.Peers[0].State != "established" {
		t.Errorf("peer0 state = %q, want established", s.Peers[0].State)
	}
	if s.Peers[0].PrefixesReceived != 500 ||
		s.Peers[0].PrefixesAccepted != 450 ||
		s.Peers[0].PrefixesSent != 600 {
		t.Errorf("peer0 pfx recv/acc/sent = %d/%d/%d",
			s.Peers[0].PrefixesReceived,
			s.Peers[0].PrefixesAccepted,
			s.Peers[0].PrefixesSent)
	}
	if s.Peers[0].AddressFamily != "IPv4" {
		t.Errorf("peer0 afi = %q", s.Peers[0].AddressFamily)
	}
	// Peer 2: Active (non-Established).
	if s.Peers[1].State != "active" {
		t.Errorf("peer1 state = %q, want active", s.Peers[1].State)
	}
	// Peer 3: v6 Established.
	if s.Peers[2].State != "established" {
		t.Errorf("peer2 state = %q, want established", s.Peers[2].State)
	}
	if s.Peers[2].AddressFamily != "IPv6" {
		t.Errorf("peer2 afi = %q", s.Peers[2].AddressFamily)
	}
}

const mikrotikBGPSummarySample = ` Flags: E - established, * - dynamic
  0 E name="peer1"
      remote.address=10.0.0.2 .as=65001 .id=10.0.0.2 .refresh=yes
      local.address=10.0.0.1 .as=65000 .role=ebgp
      output.network="bgp-networks" .filter-chain="" .accepted-routes=600
      input.filter="bgp-in" .accepted-routes=500 .rejected-routes=0 .ignored-routes=0
      hold-time=3m keepalive-time=1m uptime=1d2h3m4s
      prefix-count=500
  1 name="peer2"
      remote.address=10.0.0.3 .as=65002 .id=0.0.0.0 .refresh=no
      local.address=10.0.0.1 .as=65000 .role=ebgp
      hold-time=3m keepalive-time=1m uptime=0s
      prefix-count=0
`

func TestTextFSMParserMikroTikBGPSummary(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPSummary, []byte(mikrotikBGPSummarySample),
		Config{Template: "mikrotik_routeros_show_bgp_summary"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	s, ok := r.Payload.(*pb.BGPSummaryParsed)
	if !ok {
		t.Fatalf("Payload type = %T", r.Payload)
	}
	if got := len(s.Peers); got != 2 {
		t.Fatalf("len(Peers) = %d, want 2", got)
	}
	// Peer 1: `E` flag → projector → "established".
	if s.Peers[0].State != "established" {
		t.Errorf("peer0 state = %q, want established", s.Peers[0].State)
	}
	if s.Peers[0].PeerAsn != 65001 {
		t.Errorf("peer0 asn = %d", s.Peers[0].PeerAsn)
	}
	if s.Peers[0].PrefixesReceived != 500 || s.Peers[0].PrefixesSent != 600 {
		t.Errorf("peer0 pfx recv/sent = %d/%d",
			s.Peers[0].PrefixesReceived, s.Peers[0].PrefixesSent)
	}
	// Peer 2: no `E` flag → state empty.
	if s.Peers[1].State != "" {
		t.Errorf("peer1 state = %q, want empty", s.Peers[1].State)
	}
	if s.Peers[1].PeerAsn != 65002 {
		t.Errorf("peer1 asn = %d", s.Peers[1].PeerAsn)
	}
}

const aristaBGPPathsEmptySample = `BGP routing table information for VRF PROUD-BEAVER
Router identifier 0.0.0.4, local AS number 214503

BGP routing table information for VRF PROUD-BEAVER
Router identifier 0.0.0.4, local AS number 214503

`

// TestTextFSMParserAristaBGPPathsEmpty asserts that a zero-match
// BGP lookup returns OK with an empty paths slice.
func TestTextFSMParserAristaBGPPathsEmpty(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPCommunity, []byte(aristaBGPPathsEmptySample),
		Config{Template: "arista_eos_show_bgp_paths"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK (zero-match BGP lookup must not flip to PARSE_FAILED)", r.Status)
	}
	if r.Kind != pb.ParserKind_PARSER_KIND_TEXTFSM {
		t.Errorf("Kind = %v, want TEXTFSM", r.Kind)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T, want *pb.BGPPaths", r.Payload)
	}
	if got := len(paths.Paths); got != 0 {
		t.Errorf("len(Paths) = %d, want 0 (this sample has no per-path blocks)", got)
	}
}

const aristaBGPSummaryEmptySample = `BGP summary information for VRF PROUD-BEAVER
Router identifier 0.0.0.4, local AS number 214503
Neighbor Status Codes: m - Under maintenance
  Description              Neighbor         V AS           MsgRcvd   MsgSent  InQ OutQ  Up/Down State   PfxRcd PfxAcc

`

// TestTextFSMParserAristaBGPSummaryEmpty asserts that a header-only
// summary returns OK with an empty peers slice.
func TestTextFSMParserAristaBGPSummaryEmpty(t *testing.T) {
	r := TextFSMParser{}.Parse(OpBGPSummary, []byte(aristaBGPSummaryEmptySample),
		Config{Template: "arista_eos_show_bgp_summary"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK (zero-peer summary must not flip to PARSE_FAILED)", r.Status)
	}
	if r.Kind != pb.ParserKind_PARSER_KIND_TEXTFSM {
		t.Errorf("Kind = %v, want TEXTFSM", r.Kind)
	}
	summary, ok := r.Payload.(*pb.BGPSummaryParsed)
	if !ok {
		t.Fatalf("Payload type = %T, want *pb.BGPSummaryParsed", r.Payload)
	}
	if got := len(summary.Peers); got != 0 {
		t.Errorf("len(Peers) = %d, want 0", got)
	}
}

// equalU32 reports whether two []uint32 slices are equal.
func equalU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// equalStr reports whether two []string slices are equal.
func equalStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
