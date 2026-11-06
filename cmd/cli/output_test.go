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
