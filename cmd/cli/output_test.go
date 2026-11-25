package main

import (
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/AS203038/looking-glass/pkg/routers/parse"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

// captureStdout returns what fn writes to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestPrintTraceroutePrettyLostFirstProbe(t *testing.T) {
	out := ` 1  * rtr-a.example (192.0.2.1)  3.284 ms  1.683 ms
 2  * * 192.0.2.5  0.390 ms
`
	r := parse.TextFSMParser{}.Parse(parse.OpTraceroute, []byte(out), parse.Config{Template: "juniper_junos_traceroute"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	got := captureStdout(t, func() { printTraceroutePretty(r.Payload.(*pb.TracerouteParsed)) })
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want header and 2 hops:\n%s", len(lines), got)
	}
	// The host and address come from the first probe that answered.
	for i, want := range [][]string{{"1", "rtr-a.example", "192.0.2.1"}, {"2", "—", "192.0.2.5"}} {
		if f := strings.Fields(lines[i+1]); len(f) < 3 || !slices.Equal(f[:3], want) {
			t.Errorf("hop %d = %q, want TTL, HOST, IP %q", i+1, lines[i+1], want)
		}
	}
}

// A reply can have an RTT of 0: Cisco IOS prints "0 msec" for a near hop.
func TestPrintTraceroutePrettyZeroRTTReply(t *testing.T) {
	out := `  1 10.0.0.254 0 msec 0 msec 0 msec
  2 * * *
`
	r := parse.TextFSMParser{}.Parse(parse.OpTraceroute, []byte(out), parse.Config{Template: "cisco_ios_traceroute"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	got := captureStdout(t, func() { printTraceroutePretty(r.Payload.(*pb.TracerouteParsed)) })
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want header and 2 hops:\n%s", len(lines), got)
	}
	if f := strings.Fields(lines[1]); len(f) < 5 || !slices.Equal(f[:5], []string{"1", "—", "10.0.0.254", "0.0", "ms"}) {
		t.Errorf("hop 1 = %q, want the RTT of the reply from 10.0.0.254 as 0.0 ms", lines[1])
	}
	if f := strings.Fields(lines[2]); !slices.Equal(f, []string{"2", "—", "*", "*"}) {
		t.Errorf("hop 2 = %q, want a lost probe as *", lines[2])
	}
}

// A state with a reason has the color of the state without the reason.
func TestPrintBGPSummaryPrettyStateColor(t *testing.T) {
	old := opts.ForceColor
	opts.ForceColor = true
	defer func() { opts.ForceColor = old }()

	for _, tc := range []struct{ state, color string }{
		{"established", ansiGreen},
		{"idle", ansiYellow},
		{"idle (removal in progress)", ansiYellow},
		{"idle(noif)", ansiYellow},
		{"connect", ansiYellow},
		{"active", ansiRed},
	} {
		s := &pb.BGPSummaryParsed{Peers: []*pb.BGPPeer{{PeerIp: "192.0.2.1", State: tc.state}}}
		got := captureStdout(t, func() { printBGPSummaryPretty(s) })
		if want := tc.color + tc.state + ansiReset; !strings.Contains(got, want) {
			t.Errorf("state %q: output has no %q:\n%q", tc.state, want, got)
		}
	}
}

// TestLevenshteinAndBGPPathsPretty verifies levenshtein distances, colorTabWriter stripping/coloring, and printBGPPathsPretty formatting.
func TestLevenshteinAndBGPPathsPretty(t *testing.T) {
	if got := levenshtein("kitten", "sitting"); got != 3 {
		t.Errorf("levenshtein(kitten, sitting) = %d, want 3", got)
	}
	if got := levenshtein("", "abc"); got != 3 || levenshtein("abc", "") != 3 {
		t.Errorf("levenshtein empty check failed")
	}

	paths := &pb.BGPPaths{
		Paths: []*pb.BGPPath{
			{
				Best:             true,
				Prefix:           "192.0.2.0/24",
				Nexthop:          "192.0.2.1",
				Med:              10,
				LocalPref:        100,
				AsPath:           []uint32{65001, 203038},
				PeerAsn:          65001,
				PeerIp:           "192.0.2.1",
				Communities:      []string{"65001:100"},
				LargeCommunities: []string{"65001:100:1"},
			},
		},
	}

	oldNoColor := opts.NoColor
	opts.NoColor = true
	defer func() { opts.NoColor = oldNoColor }()

	got := captureStdout(t, func() { printBGPPathsPretty(paths) })
	if strings.Contains(got, "\x1b[") {
		t.Errorf("expected ANSI escapes stripped when NoColor=true, got %q", got)
	}
	if !strings.Contains(got, "65001 203038") || !strings.Contains(got, "65001:100 65001:100:1") {
		t.Errorf("unexpected printBGPPathsPretty output: %q", got)
	}
}
