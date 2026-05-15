package parse

import (
	"log"
	"regexp"
	"strconv"
	"strings"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"google.golang.org/protobuf/proto"
)

// BuiltinParser is a small, hand-rolled parser for outputs whose
// structure is trivial enough that a full TextFSM template would be
// overkill. It currently covers the Linux iputils tooling used by
// FRRouting (ping, traceroute) — both formats have been stable for
// decades and the parser is only a few dozen lines.
//
// New op handlers should be added sparingly: the moment a format
// starts varying by version or vendor, TextFSM is the right tool.
type BuiltinParser struct{}

// Name implements [Parser].
func (BuiltinParser) Name() string { return "builtin" }

// Parse implements [Parser] by fanning out on op.
func (p BuiltinParser) Parse(op Op, raw []byte, _ Config) Result {
	switch op {
	case OpPing:
		stats := parseLinuxPing(raw)
		if stats == nil {
			return Failed(pb.ParserKind_PARSER_KIND_BUILTIN)
		}
		return OK(pb.ParserKind_PARSER_KIND_BUILTIN, proto.Message(stats))
	case OpTraceroute:
		tp := parseLinuxTraceroute(raw)
		if tp == nil {
			return Failed(pb.ParserKind_PARSER_KIND_BUILTIN)
		}
		return OK(pb.ParserKind_PARSER_KIND_BUILTIN, proto.Message(tp))
	default:
		log.Printf("PARSE: builtin parser has no handler for op=%s", op)
		return Failed(pb.ParserKind_PARSER_KIND_BUILTIN)
	}
}

// ---------------------------------------------------------------------------
// Linux iputils ping
// ---------------------------------------------------------------------------
//
// Canonical iputils output (5 probes, no name resolution):
//
//	PING 1.1.1.1 (1.1.1.1) from 10.0.0.1 : 56(84) bytes of data.
//	64 bytes from 1.1.1.1: icmp_seq=1 ttl=58 time=2.10 ms
//	...
//	--- 1.1.1.1 ping statistics ---
//	5 packets transmitted, 5 received, 0% packet loss, time 4006ms
//	rtt min/avg/max/mdev = 1.987/2.123/2.456/0.156 ms

var (
	pingHeaderRE = regexp.MustCompile(`(?m)^PING\s+\S+\s+\(([0-9a-fA-F:.]+)\)(?:\s+from\s+([0-9a-fA-F:.]+))?`)
	pingHeader6  = regexp.MustCompile(`(?m)^PING\s+([0-9a-fA-F:.]+)\(([0-9a-fA-F:.]+)\)(?:\s+from\s+([0-9a-fA-F:.]+))?`)
	pingStatsRE  = regexp.MustCompile(`(?m)^(\d+)\s+packets transmitted,\s+(\d+)\s+received(?:,\s+(?:\+\d+\s+errors,\s+)?(?:(\d+(?:\.\d+)?)%\s+packet loss))?`)
	pingRTTRE    = regexp.MustCompile(`(?m)^(?:rtt|round-trip)\s+min/avg/max/(?:mdev|stddev)\s+=\s+(\d+(?:\.\d+)?)/(\d+(?:\.\d+)?)/(\d+(?:\.\d+)?)/(\d+(?:\.\d+)?)\s+ms`)
)

// parseLinuxPing extracts a [pb.PingStats] from raw iputils ping
// output. Returns nil when no statistics block was found (probable
// command failure — the caller will surface PARSE_FAILED and the
// raw `result` bytes remain authoritative).
func parseLinuxPing(raw []byte) *pb.PingStats {
	text := string(raw)
	stats := &pb.PingStats{}

	// Header — preferred form first (ping `-4`), then the `-6` form.
	if m := pingHeaderRE.FindStringSubmatch(text); len(m) == 3 {
		stats.Target = m[1]
		if m[2] != "" {
			stats.Source = m[2]
		}
	} else if m := pingHeader6.FindStringSubmatch(text); len(m) == 4 {
		stats.Target = m[2]
		if m[3] != "" {
			stats.Source = m[3]
		}
	}

	// Statistics line. If we don't find it the run effectively
	// failed; bail with nil so the caller falls back to raw.
	m := pingStatsRE.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	if v, err := strconv.ParseUint(m[1], 10, 32); err == nil {
		stats.PacketsSent = uint32(v)
	}
	if v, err := strconv.ParseUint(m[2], 10, 32); err == nil {
		stats.PacketsReceived = uint32(v)
	}
	if m[3] != "" {
		if v, err := strconv.ParseFloat(m[3], 32); err == nil {
			stats.LossPct = float32(v)
		}
	} else if stats.PacketsSent > 0 {
		// Derive loss% when the line omitted it (rare on modern
		// iputils but seen on some embedded builds).
		stats.LossPct = float32(stats.PacketsSent-stats.PacketsReceived) /
			float32(stats.PacketsSent) * 100
	}

	// RTT summary — optional (absent on 100% loss).
	if m := pingRTTRE.FindStringSubmatch(text); m != nil {
		if v, err := strconv.ParseFloat(m[1], 32); err == nil {
			stats.RttMinMs = float32(v)
		}
		if v, err := strconv.ParseFloat(m[2], 32); err == nil {
			stats.RttAvgMs = float32(v)
		}
		if v, err := strconv.ParseFloat(m[3], 32); err == nil {
			stats.RttMaxMs = float32(v)
		}
		if v, err := strconv.ParseFloat(m[4], 32); err == nil {
			stats.RttMdevMs = float32(v)
		}
	}
	return stats
}

// ---------------------------------------------------------------------------
// Linux iputils traceroute
// ---------------------------------------------------------------------------
//
// Canonical iputils output (one probe per hop, -q1, name resolution
// enabled, FRR template adds -e/--mtu/--back decorations which we
// silently tolerate):
//
//	traceroute to 1.1.1.1 (1.1.1.1), 30 hops max, 60 byte packets
//	 1  hop1.example.com (10.0.0.1)  0.234 ms
//	 2  *
//	 3  one.one.one.one (1.1.1.1)  10.123 ms
//
// With multiple probes (-q3) each timing repeats:
//
//	 1  hop1 (10.0.0.1)  0.2 ms  0.3 ms  0.4 ms

var (
	tracerouteHeaderRE = regexp.MustCompile(`(?m)^traceroute\s+to\s+\S+\s+\(([0-9a-fA-F:.]+)\)(?:\s+from\s+([0-9a-fA-F:.]+))?`)
	// hopLineRE matches the leading "<ttl>  <rest>" form. The rest
	// is parsed by the per-probe extractor below to handle the wide
	// variety of decorations FRR's traceroute may emit (-e flags,
	// MTU annotations, !N / !H / !X markers, back-path).
	hopLineRE = regexp.MustCompile(`^\s*(\d+)\s+(.*)$`)
	// probeRE matches one probe: optional "host (ip)" or "ip", an
	// optional run of decorations (back-path quote "'-N'", MTU
	// annotation "F=NNNN", ICMP-error markers "!N", "!H", "!X" …)
	// and then "<float> ms". Stars ("*") are handled separately as
	// "timeout" probes with no IP.
	probeRE = regexp.MustCompile(`(?:([\w.\-]+)\s+\(([0-9a-fA-F:.]+)\)|([0-9a-fA-F:.]+))(?:\s+'[^']*'|\s+F=\d+|\s+!\w+)*\s+(\d+(?:\.\d+)?)\s+ms`)
)

// parseLinuxTraceroute extracts a [pb.TracerouteParsed] from raw
// iputils traceroute output. Returns nil when not even the header
// could be located (almost certainly a command failure).
func parseLinuxTraceroute(raw []byte) *pb.TracerouteParsed {
	text := string(raw)
	tp := &pb.TracerouteParsed{}

	if m := tracerouteHeaderRE.FindStringSubmatch(text); len(m) >= 2 {
		tp.Target = m[1]
		if len(m) > 2 && m[2] != "" {
			tp.Source = m[2]
		}
	} else {
		return nil
	}

	for _, line := range strings.Split(text, "\n") {
		m := hopLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ttl64, err := strconv.ParseUint(m[1], 10, 32)
		if err != nil {
			continue
		}
		hop := &pb.TracerouteHop{Ttl: uint32(ttl64)}
		rest := m[2]
		// Count timeouts ("*"): when every probe slot is a "*" the
		// hop carries one empty Probe with rtt_ms=0; otherwise we
		// emit one Probe per "<host (ip)> <rtt> ms" match.
		hadProbe := false
		for _, p := range probeRE.FindAllStringSubmatch(rest, -1) {
			probe := &pb.TracerouteProbe{}
			if p[1] != "" {
				probe.Hostname = p[1]
				probe.Ip = p[2]
			} else {
				probe.Ip = p[3]
			}
			if v, err := strconv.ParseFloat(p[4], 32); err == nil {
				probe.RttMs = float32(v)
			}
			hop.Probes = append(hop.Probes, probe)
			hadProbe = true
		}
		if !hadProbe {
			// All stars → one synthetic timeout probe so the UI
			// can render an explicit "no response" row instead of
			// silently skipping the hop.
			hop.Probes = append(hop.Probes, &pb.TracerouteProbe{})
		}
		tp.Hops = append(tp.Hops, hop)
	}

	if len(tp.Hops) == 0 {
		return nil
	}
	return tp
}
