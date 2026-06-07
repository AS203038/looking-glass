package parse

import (
	"testing"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

const frrBGPRouteSample = `{
  "vrfName": "default",
  "routerId": "10.0.0.1",
  "routes": {
    "1.1.1.0/24": [
      {
        "valid": true,
        "bestpath": {"overall": true},
        "med": 0,
        "locPrf": 100,
        "aspath": "65001 65002 13335",
        "origin": "IGP",
        "community": {"string": "65000:100 65000:200"},
        "largeCommunity": {"string": "214503:8:3607"},
        "nexthops": [{"ip": "10.0.0.2", "afi": "ipv4"}],
        "peer": {"peerId": "10.0.0.2", "asn": 65001},
        "uptime": "1d02h"
      }
    ]
  }
}`

const frrBGPRouteDual = frrBGPRouteSample + "\n" + `{
  "vrfName": "default",
  "routerId": "10.0.0.1",
  "routes": {
    "2606:4700:4700::/48": [
      {
        "valid": true,
        "bestpath": {"overall": true},
        "aspath": "13335",
        "origin": "IGP",
        "nexthops": [{"ip": "fe80::1", "afi": "ipv6"}],
        "peer": {"peerId": "fe80::1", "asn": 13335},
        "uptime": "5w"
      }
    ]
  }
}`

const frrBGPSummarySample = `{
  "ipv4Unicast": {
    "routerId": "10.0.0.1",
    "as": 65000,
    "peers": {
      "10.0.0.2": {
        "remoteAs": 65001,
        "state": "Established",
        "peerUptimeMsec": 1234567,
        "pfxRcd": 100,
        "pfxSnt": 200
      }
    }
  },
  "ipv6Unicast": {
    "routerId": "10.0.0.1",
    "as": 65000,
    "peers": {
      "fe80::1": {
        "remoteAs": 13335,
        "state": "Established",
        "peerUptimeMsec": 9876543,
        "pfxRcd": 50,
        "pfxSnt": 60
      }
    }
  }
}`

// TestJSONParserName verifies the JSONParser's name and error methods.
func TestJSONParserName(t *testing.T) {
	p := JSONParser{}
	if p.Name() != "native_json" {
		t.Errorf("expected native_json, got %q", p.Name())
	}
}

// TestJSONParserFRRRoute verifies single-envelope decoding and the
// AS-path projection.
func TestJSONParserFRRRoute(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPRoute, []byte(frrBGPRouteSample),
		Config{Schema: "frr_bgp_route_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok || len(paths.Paths) != 1 {
		t.Fatalf("Paths = %v", paths)
	}
	p := paths.Paths[0]
	if p.Prefix != "1.1.1.0/24" || p.Nexthop != "10.0.0.2" {
		t.Errorf("prefix/nexthop: %+v", p)
	}
	if !p.Best || p.LocalPref != 100 || p.Origin != "igp" {
		t.Errorf("best/locpref/origin: %+v", p)
	}
	if len(p.AsPath) != 3 || p.AsPath[0] != 65001 || p.AsPath[2] != 13335 {
		t.Errorf("as_path: %v", p.AsPath)
	}
	if len(p.Communities) != 2 || p.Communities[0] != "65000:100" {
		t.Errorf("communities: %v", p.Communities)
	}
	if len(p.LargeCommunities) != 1 || p.LargeCommunities[0] != "214503:8:3607" {
		t.Errorf("large_communities: %v", p.LargeCommunities)
	}
	if p.PeerAsn != 65001 || p.PeerIp != "10.0.0.2" {
		t.Errorf("peer: %+v", p)
	}
	// 1d02h = 86400 + 2*3600 = 93600
	if p.AgeSeconds != 93600 {
		t.Errorf("AgeSeconds = %d, want 93600", p.AgeSeconds)
	}
}

// TestJSONParserFRRDualEnvelope verifies splitting and decoding of
// two concatenated top-level envelopes.
func TestJSONParserFRRDualEnvelope(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPCommunity, []byte(frrBGPRouteDual),
		Config{Schema: "frr_bgp_route_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v", r.Status)
	}
	paths := r.Payload.(*pb.BGPPaths)
	if len(paths.Paths) != 2 {
		t.Fatalf("len(Paths) = %d, want 2", len(paths.Paths))
	}
	// 5w = 5*7*86400
	gotV6 := paths.Paths[0]
	if gotV6.Prefix != "1.1.1.0/24" {
		// Order may differ; find the v6 one
		gotV6 = paths.Paths[1]
	}
	if gotV6.Prefix != "2606:4700:4700::/48" && paths.Paths[1].Prefix != "2606:4700:4700::/48" {
		t.Errorf("expected one v6 prefix, got %v", paths.Paths)
	}
}

// TestJSONParserFRRSummary verifies the wrapped multi-AFI summary
// shape projects into a flat peer list with AFI labels attached.
func TestJSONParserFRRSummary(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPSummary, []byte(frrBGPSummarySample),
		Config{Schema: "frr_bgp_summary_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v", r.Status)
	}
	summary := r.Payload.(*pb.BGPSummaryParsed)
	if summary.LocalAsn != 65000 || summary.RouterId != "10.0.0.1" {
		t.Errorf("local_asn/router_id: %+v", summary)
	}
	if len(summary.Peers) != 2 {
		t.Fatalf("len(Peers) = %d, want 2", len(summary.Peers))
	}
	gotV4, gotV6 := false, false
	for _, p := range summary.Peers {
		switch p.AddressFamily {
		case "ipv4-unicast":
			gotV4 = true
			if p.PeerIp != "10.0.0.2" || p.PeerAsn != 65001 || p.State != "established" ||
				p.UptimeSeconds != 1234 || p.PrefixesReceived != 100 {
				t.Errorf("v4 peer: %+v", p)
			}
		case "ipv6-unicast":
			gotV6 = true
			if p.PeerIp != "fe80::1" || p.PeerAsn != 13335 {
				t.Errorf("v6 peer: %+v", p)
			}
		}
	}
	if !gotV4 || !gotV6 {
		t.Errorf("missing AFI: v4=%v v6=%v", gotV4, gotV6)
	}
}

// TestJSONParserBadInput verifies that malformed JSON returns
// PARSE_FAILED.
func TestJSONParserBadInput(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPRoute, []byte("not json at all"), Config{})
	if r.Status != pb.ParseStatus_PARSE_STATUS_PARSE_FAILED {
		t.Errorf("Status = %v, want PARSE_FAILED", r.Status)
	}
}

const frrBGPSummaryDirectV4 = `{
  "routerId": "10.0.0.1",
  "as": 65000,
  "vrfId": 0,
  "vrfName": "default",
  "peers": {
    "10.0.0.2": {
      "remoteAs": 65001,
      "state": "Established",
      "peerUptimeMsec": 1234567,
      "pfxRcd": 100,
      "pfxSnt": 200
    }
  }
}`

const frrBGPSummaryDirectV6 = `{
  "routerId": "10.0.0.1",
  "as": 65000,
  "vrfId": 0,
  "vrfName": "default",
  "peers": {
    "fe80::1": {
      "remoteAs": 13335,
      "state": "Established",
      "peerUptimeMsec": 9876543,
      "pfxRcd": 50,
      "pfxSnt": 60
    }
  }
}`

// TestJSONParserFRRSummaryDirect verifies decoding of the direct
// per-AFI summary shape and merging of two concatenated envelopes.
func TestJSONParserFRRSummaryDirect(t *testing.T) {
	combined := frrBGPSummaryDirectV4 + "\n" + frrBGPSummaryDirectV6
	r := JSONParser{}.Parse(OpBGPSummary, []byte(combined),
		Config{Schema: "frr_bgp_summary_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	summary := r.Payload.(*pb.BGPSummaryParsed)
	if summary.LocalAsn != 65000 || summary.RouterId != "10.0.0.1" {
		t.Errorf("local_asn/router_id: %+v", summary)
	}
	if len(summary.Peers) != 2 {
		t.Fatalf("len(Peers) = %d, want 2", len(summary.Peers))
	}
	gotV4, gotV6 := false, false
	for _, p := range summary.Peers {
		switch p.PeerIp {
		case "10.0.0.2":
			gotV4 = true
			if p.AddressFamily != "ipv4-unicast" || p.State != "established" ||
				p.UptimeSeconds != 1234 || p.PrefixesReceived != 100 {
				t.Errorf("v4 peer: %+v", p)
			}
		case "fe80::1":
			gotV6 = true
			if p.AddressFamily != "ipv6-unicast" || p.PeerAsn != 13335 {
				t.Errorf("v6 peer: %+v", p)
			}
		}
	}
	if !gotV4 || !gotV6 {
		t.Errorf("missing AFI: v4=%v v6=%v", gotV4, gotV6)
	}
}

const frrBGPRouteSinglePrefix = `{
  "prefix": "1.1.1.0/24",
  "paths": [
    {
      "valid": true,
      "bestpath": {"overall": true},
      "metric": 0,
      "locPrf": 100,
      "aspath": {
        "string": "65001 65002 13335",
        "segments": [],
        "length": 3
      },
      "origin": "IGP",
      "community": {"string": "65000:100 65000:200"},
      "largeCommunity": {"string": "214503:8:3607"},
      "nexthops": [{"ip": "10.0.0.2", "afi": "ipv4"}],
      "peerId": "10.0.0.2",
      "lastUpdate": {
        "epoch": 1700000000,
        "string": "1d02h"
      }
    }
  ]
}`

// TestJSONParserFRRRouteSinglePrefix verifies decoding of the
// FRR v9.x+ single-prefix detail shape.
func TestJSONParserFRRRouteSinglePrefix(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPRoute, []byte(frrBGPRouteSinglePrefix),
		Config{Schema: "frr_bgp_route_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths := r.Payload.(*pb.BGPPaths)
	if len(paths.Paths) != 1 {
		t.Fatalf("len(Paths) = %d, want 1", len(paths.Paths))
	}
	p := paths.Paths[0]
	if p.Prefix != "1.1.1.0/24" {
		t.Errorf("Prefix = %q", p.Prefix)
	}
	if len(p.AsPath) != 3 || p.AsPath[0] != 65001 || p.AsPath[2] != 13335 {
		t.Errorf("AsPath = %v, want [65001 65002 13335]", p.AsPath)
	}
	if p.PeerIp != "10.0.0.2" {
		t.Errorf("PeerIp = %q (expected single-prefix peerId to be honoured)", p.PeerIp)
	}
	// 1d02h = 86400 + 2*3600 = 93600
	if p.AgeSeconds != 93600 {
		t.Errorf("AgeSeconds = %d, want 93600 (from lastUpdate.string)", p.AgeSeconds)
	}
	if !p.Best || p.LocalPref != 100 || p.Origin != "igp" {
		t.Errorf("best/locpref/origin: %+v", p)
	}
}

const frrBGPRouteV9MultiPrefix = `{
  "vrfName": "default",
  "routerId": "10.0.0.1",
  "routes": {
    "8.8.8.0/24": [
      {
        "valid": true,
        "bestpath": {"overall": true},
        "med": 0,
        "locPrf": 100,
        "aspath": {"string": "15169", "segments": [], "length": 1},
        "origin": "IGP",
        "nexthops": [{"ip": "10.0.0.2", "afi": "ipv4"}],
        "peer": {"peerId": "10.0.0.2", "asn": 65001},
        "uptime": "5w"
      }
    ]
  }
}`

const frrBGPCommunityRealSample = `{
  "vrfId": 12,
  "vrfName": "IBGP",
  "tableVersion": 85418408,
  "routerId": "185.243.23.254",
  "defaultLocPrf": 100,
  "localAS": 203038,
  "routes": {
    "1.0.0.0/24": [
      {
        "valid": true,
        "bestpath": true,
        "selectionReason": "Local Pref",
        "pathFrom": "external",
        "prefix": "1.0.0.0",
        "prefixLen": 24,
        "network": "1.0.0.0/24",
        "version": 28058480,
        "metric": 100,
        "locPrf": 200,
        "weight": 0,
        "peerId": "(unspec)",
        "path": "13335",
        "origin": "IGP",
        "announceNexthopSelf": true,
        "nhVrfName": "IX_SOLIX",
        "nhVrfId": 14,
        "nexthops": [
          {
            "ip": "193.110.13.152",
            "hostname": "rt1.sto1.se.as203038.net",
            "afi": "ipv4",
            "used": true
          }
        ]
      }
    ]
  }
}`

// TestJSONParserFRRCommunityRealSample verifies decoding of an FRR
// envelope using bare-bool `bestpath` and the `path` AS-path key.
func TestJSONParserFRRCommunityRealSample(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPCommunity, []byte(frrBGPCommunityRealSample),
		Config{Schema: "frr_bgp_route_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK (regression: parser used to bail on bestpath=true)", r.Status)
	}
	paths := r.Payload.(*pb.BGPPaths)
	if len(paths.Paths) != 1 {
		t.Fatalf("len(Paths) = %d, want 1", len(paths.Paths))
	}
	p := paths.Paths[0]
	if p.Prefix != "1.0.0.0/24" {
		t.Errorf("Prefix = %q", p.Prefix)
	}
	if !p.Best {
		t.Errorf("Best = false, want true (bestpath: true is the bare-bool form)")
	}
	if len(p.AsPath) != 1 || p.AsPath[0] != 13335 {
		t.Errorf("AsPath = %v, want [13335] (from \"path\" key, not \"aspath\")", p.AsPath)
	}
	if p.Nexthop != "193.110.13.152" {
		t.Errorf("Nexthop = %q, want 193.110.13.152", p.Nexthop)
	}
	if p.Med != 100 {
		t.Errorf("Med = %d, want 100 (from metric)", p.Med)
	}
	if p.LocalPref != 200 {
		t.Errorf("LocalPref = %d, want 200", p.LocalPref)
	}
}

func TestJSONParserFRRRouteV9ASPathObject(t *testing.T) {

	r := JSONParser{}.Parse(OpBGPRoute, []byte(frrBGPRouteV9MultiPrefix),
		Config{Schema: "frr_bgp_route_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK", r.Status)
	}
	paths := r.Payload.(*pb.BGPPaths)
	if len(paths.Paths) != 1 {
		t.Fatalf("len(Paths) = %d, want 1", len(paths.Paths))
	}
	p := paths.Paths[0]
	if len(p.AsPath) != 1 || p.AsPath[0] != 15169 {
		t.Errorf("AsPath = %v, want [15169] (from aspath object)", p.AsPath)
	}
	// 5w = 5 * 7 * 86400 = 3024000
	if p.AgeSeconds != 3024000 {
		t.Errorf("AgeSeconds = %d, want 3024000", p.AgeSeconds)
	}
}

const frrBGPEmptyResultsDual = `{
 "vrfId": 12,
 "vrfName": "IBGP",
 "tableVersion": 197174680,
 "routerId": "185.243.23.255",
 "defaultLocPrf": 100,
 "localAS": 203038,
 "routes": {  }  } 

{
 "vrfId": 12,
 "vrfName": "IBGP",
 "tableVersion": 130636276,
 "routerId": "185.243.23.255",
 "defaultLocPrf": 100,
 "localAS": 203038,
 "routes": {  }  } 
`

// TestJSONParserFRREmptyResultsEnvelope verifies that an envelope
// with an empty `routes` map returns OK with no paths.
func TestJSONParserFRREmptyResultsEnvelope(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPCommunity, []byte(frrBGPEmptyResultsDual),
		Config{Schema: "frr_bgp_route_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK (empty routes-map must not flip to PARSE_FAILED)",
			r.Status)
	}
	if r.Kind != pb.ParserKind_PARSER_KIND_NATIVE_JSON {
		t.Errorf("Kind = %v, want NATIVE_JSON", r.Kind)
	}
	paths, ok := r.Payload.(*pb.BGPPaths)
	if !ok {
		t.Fatalf("Payload type = %T, want *pb.BGPPaths", r.Payload)
	}
	if got := len(paths.Paths); got != 0 {
		t.Errorf("len(Paths) = %d, want 0", got)
	}
}

const frrBGPSummaryEmptyPeersV4 = `{
  "routerId": "10.0.0.1",
  "as": 65000,
  "vrfId": 0,
  "vrfName": "default",
  "peers": { }
}`

// TestJSONParserFRRSummaryEmptyPeers verifies that a summary
// envelope with an empty `peers` map returns OK with no peer rows.
func TestJSONParserFRRSummaryEmptyPeers(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPSummary, []byte(frrBGPSummaryEmptyPeersV4),
		Config{Schema: "frr_bgp_summary_v1"})
	if r.Status != pb.ParseStatus_PARSE_STATUS_OK {
		t.Fatalf("Status = %v, want OK (empty peers map must not flip to PARSE_FAILED)",
			r.Status)
	}
	summary, ok := r.Payload.(*pb.BGPSummaryParsed)
	if !ok {
		t.Fatalf("Payload type = %T, want *pb.BGPSummaryParsed", r.Payload)
	}
	if got := len(summary.Peers); got != 0 {
		t.Errorf("len(Peers) = %d, want 0", got)
	}
	// Header context still propagates even when the peer list is
	// empty — useful diagnostic for the WebUI/CLI even on a quiet
	// router.
	if summary.LocalAsn != 65000 {
		t.Errorf("LocalAsn = %d, want 65000", summary.LocalAsn)
	}
	if summary.RouterId != "10.0.0.1" {
		t.Errorf("RouterId = %q, want 10.0.0.1", summary.RouterId)
	}
}

func TestFRRDurationHelper(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"00:00:05", 5},
		{"01:02:03", 3723},
		{"1d02h", 93600},
		{"5w", 3024000},
		{"invalid", 0},
		{"xyz:abc:def", 0}, // triggers error inside Count == 2 colons block without matching any unit suffix
		{"", 0},            // triggers s == ""
	}
	for _, tc := range cases {
		got := parseFRRDuration(tc.in)
		if got != tc.want {
			t.Errorf("parseFRRDuration(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestFscanCountHelper(t *testing.T) {
	var val uint64
	n, err := fscanCount("123", &val)
	if err != nil || n != 1 || val != 123 {
		t.Errorf("expected 123, got %d (err: %v, n: %d)", val, err, n)
	}

	// s has more fields than dsts (covers i >= len(dsts))
	var val2 uint64
	n, err = fscanCount("123:456", &val2)
	if err != nil || n != 1 || val2 != 123 {
		t.Errorf("expected 123, got %d (err: %v, n: %d)", val2, err, n)
	}

	// s has invalid non-numeric fields (covers strconv.ParseUint error)
	var val3 uint64
	_, err = fscanCount("invalid", &val3)
	if err == nil {
		t.Errorf("expected error on invalid fscanCount")
	}
}

func TestJSONParserEdgeCases(t *testing.T) {
	// 1. Unsupported operation fallback
	res := JSONParser{}.Parse(OpPing, []byte(frrBGPRouteSample), Config{Schema: "frr_bgp_route_v1"})
	if res.Status != pb.ParseStatus_PARSE_STATUS_TEMPLATE_MISSING {
		t.Errorf("expected TEMPLATE_MISSING for unsupported op, got %v", res.Status)
	}

	// 2. Malformed JSON input
	res2 := JSONParser{}.Parse(OpBGPRoute, []byte("invalid json"), Config{})
	if res2.Status != pb.ParseStatus_PARSE_STATUS_PARSE_FAILED {
		t.Errorf("expected FAILED for invalid JSON, got %v", res2.Status)
	}

	res3 := JSONParser{}.Parse(OpBGPSummary, []byte("invalid json"), Config{})
	if res3.Status != pb.ParseStatus_PARSE_STATUS_PARSE_FAILED {
		t.Errorf("expected FAILED for invalid summary JSON, got %v", res3.Status)
	}

	// 3. Cover stringErr.Error()
	errStr := errParseDuration.Error()
	if errStr != "parse duration" {
		t.Errorf("unexpected stringErr, got %q", errStr)
	}
}
