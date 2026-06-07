package parse

import (
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/AS203038/looking-glass/pkg/logging"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"google.golang.org/protobuf/proto"
)

// parseLog is the component-tagged logger shared by every parser in this
// package.
var parseLog = logging.Component("parse")

// BuiltinParser is the [Parser] for Linux iputils ping and
// traceroute output (used by FRRouting).
type BuiltinParser struct{}

// Name implements [Parser].
func (BuiltinParser) Name() string { return "builtin" }

// Parse implements [Parser] by fanning out on op.
func (p BuiltinParser) Parse(op Op, raw []byte, _ Config) Result {
	parseLog.Debug("parser run",
		slog.String("parser", "builtin"),
		slog.String("op", string(op)),
		slog.Int("raw_bytes", len(raw)))
	switch op {
	case OpPing:
		stats := parseLinuxPing(raw)
		if stats == nil {
			return Failed(pb.ParserKind_PARSER_KIND_BUILTIN)
		}
		parseLog.Debug("parser ok",
			slog.String("parser", "builtin"),
			slog.String("op", string(op)))
		return OK(pb.ParserKind_PARSER_KIND_BUILTIN, proto.Message(stats))
	case OpTraceroute:
		tp := parseLinuxTraceroute(raw)
		if tp == nil {
			return Failed(pb.ParserKind_PARSER_KIND_BUILTIN)
		}
		parseLog.Debug("parser ok",
			slog.String("parser", "builtin"),
			slog.String("op", string(op)),
			slog.Int("hops", len(tp.Hops)))
		return OK(pb.ParserKind_PARSER_KIND_BUILTIN, proto.Message(tp))
	default:
		parseLog.Warn("builtin parser has no handler",
			slog.String("op", string(op)))
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
	if len(raw) > 16384 {
		return nil
	}
	text := string(raw)
	if strings.Count(text, "\n") > 128 {
		return nil
	}
	for _, line := range strings.Split(text, "\n") {
		if len(line) > 512 {
			return nil
		}
	}
	if !strings.Contains(text, "packets transmitted") {
		return nil
	}
	stats := &pb.PingStats{}

	hasHeader := strings.Contains(text, "PING")
	if hasHeader {
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

	hasRTT := strings.Contains(text, "rtt") || strings.Contains(text, "round-trip")
	if hasRTT {
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
	if len(raw) > 16384 {
		return nil
	}
	text := string(raw)
	if strings.Count(text, "\n") > 128 {
		return nil
	}
	for _, line := range strings.Split(text, "\n") {
		if len(line) > 512 {
			return nil
		}
	}
	if !strings.Contains(text, "traceroute to") {
		return nil
	}
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
		if len(line) > 2048 {
			continue
		}
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
