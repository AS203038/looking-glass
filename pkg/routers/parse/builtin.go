package parse

import (
	"log"
	"regexp"
	"strconv"
	"strings"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"google.golang.org/protobuf/proto"
)

// BuiltinParser is the [Parser] for Linux iputils ping and
// traceroute output (used by FRRouting).
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

var (
	pingHeaderRE = regexp.MustCompile(`(?m)^PING\s+\S+\s+\(([0-9a-fA-F:.]+)\)(?:\s+from\s+([0-9a-fA-F:.]+))?`)
	pingHeader6  = regexp.MustCompile(`(?m)^PING\s+([0-9a-fA-F:.]+)\(([0-9a-fA-F:.]+)\)(?:\s+from\s+([0-9a-fA-F:.]+))?`)
	pingStatsRE  = regexp.MustCompile(`(?m)^(\d+)\s+packets transmitted,\s+(\d+)\s+received(?:,\s+(?:\+\d+\s+errors,\s+)?(?:(\d+(?:\.\d+)?)%\s+packet loss))?`)
	pingRTTRE    = regexp.MustCompile(`(?m)^(?:rtt|round-trip)\s+min/avg/max/(?:mdev|stddev)\s+=\s+(\d+(?:\.\d+)?)/(\d+(?:\.\d+)?)/(\d+(?:\.\d+)?)/(\d+(?:\.\d+)?)\s+ms`)
)

// parseLinuxPing extracts a [pb.PingStats] from raw iputils ping
// output, or nil when no statistics block was found.
func parseLinuxPing(raw []byte) *pb.PingStats {
	text := string(raw)
	stats := &pb.PingStats{}

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
		stats.LossPct = float32(stats.PacketsSent-stats.PacketsReceived) /
			float32(stats.PacketsSent) * 100
	}

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

var (
	tracerouteHeaderRE = regexp.MustCompile(`(?m)^traceroute\s+to\s+\S+\s+\(([0-9a-fA-F:.]+)\)(?:\s+from\s+([0-9a-fA-F:.]+))?`)
	hopLineRE          = regexp.MustCompile(`^\s*(\d+)\s+(.*)$`)
	probeRE            = regexp.MustCompile(`(?:([\w.\-]+)\s+\(([0-9a-fA-F:.]+)\)|([0-9a-fA-F:.]+))(?:\s+'[^']*'|\s+F=\d+|\s+!\w+)*\s+(\d+(?:\.\d+)?)\s+ms`)
)

// parseLinuxTraceroute extracts a [pb.TracerouteParsed] from raw
// iputils traceroute output, or nil when the header is missing.
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
			hop.Probes = append(hop.Probes, &pb.TracerouteProbe{})
		}
		tp.Hops = append(tp.Hops, hop)
	}

	if len(tp.Hops) == 0 {
		return nil
	}
	return tp
}
