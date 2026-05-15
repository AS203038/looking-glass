package parse

import (
	"testing"

	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
)

// FRR `show bgp ipv4 unicast 1.1.1.0/24 json` shape (abbreviated).
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

// Two top-level objects concatenated, simulating an FRR template
// that runs both `show bgp ipv4 ... json` and `show bgp ipv6 ...
// json` in a single SSH session.
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

// TestJSONParserFRRRoute covers the single-object and AS-path
// projection paths.
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

// TestJSONParserFRRDualEnvelope verifies the brace-counting
// envelope splitter handles the dual-AFI concatenation case used by
// every two-command FRR BGP template.
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

// TestJSONParserFRRSummary verifies the summary envelope projects
// each AFI's peers into one flat list with the address_family
// label attached.
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

// TestJSONParserBadInput surfaces PARSE_FAILED on malformed JSON.
func TestJSONParserBadInput(t *testing.T) {
	r := JSONParser{}.Parse(OpBGPRoute, []byte("not json at all"), Config{})
	if r.Status != pb.ParseStatus_PARSE_STATUS_PARSE_FAILED {
		t.Errorf("Status = %v, want PARSE_FAILED", r.Status)
	}
}

// FRR's `show bgp <vrf> ipv4 unicast summary json` emits the
// **direct** per-AFI form (no `ipv4Unicast` wrapper). This is what
// the bundled FRR template actually runs, so it must parse cleanly.
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

// TestJSONParserFRRSummaryDirect verifies the parser accepts the
// direct per-AFI shape (no `ipv4Unicast`/`ipv6Unicast` wrapper) and
// that two such envelopes concatenated by the dual-AFI template
// merge into one BGPSummaryParsed with AFI labels inferred from
// the peer-address family.
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

// FRR v9.x+ `show bgp ipv4 unicast 1.1.1.0/24 json` emits the
// single-prefix detail shape with `aspath` as an object, the peer
// id at the path level (`peerId`), and `lastUpdate.string` for the
// age. None of these were captured by the original parser, so
// real-router output looked structurally empty.
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

// TestJSONParserFRRRouteSinglePrefix covers the v9.x+ single-prefix
// detail shape: every field the previous parser silently dropped
// (aspath as object, path-level peerId, lastUpdate.string for age,
// `metric` instead of `med`) must now project onto the wire schema.
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

// FRR v9.x+ multi-prefix `routes` envelope where the aspath is an
// object — checks that the same path projector handles both
// envelope shapes against the v9.x aspath form.
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

// frrBGPCommunityRealSample is a real-router fragment captured
// against the AS203038 demo's `show bgp ... community ... json`.
// It exercises three shapes the original parser couldn't handle
// and that fixed-up PR5c+d code path now supports:
//
//  1. `"bestpath": true` — bare-bool form (older parser expected
//     `{"overall": true}` and got json.UnmarshalTypeError, which
//     torpedoed the whole envelope decode → PARSE_FAILED).
//  2. `"path": "13335"` — AS-path under the `path` key (older
//     parser only checked `aspath`).
//  3. Full routes-map envelope with `vrfId`/`vrfName`/`defaultLocPrf`/
//     `localAS` sibling keys — must pass through cleanly without
//     making the parser fall back to the single-prefix shape.
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

// TestJSONParserFRRCommunityRealSample is a regression test for the
// dual issue reported when FRR community/largecommunity/aspath
// queries returned raw JSON unparsed:
//   - bare-bool `bestpath`
//   - `path` key instead of `aspath`
// The parser must produce a valid BGPPaths with best=true and a
// populated as_path slice.
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

// FRR `show bgp ... community 65000:42 json` against a router that
// holds *no* prefixes tagged with that community. Both v4 and v6
// commands ran successfully; the envelope decoded cleanly; the
// `routes` map is just empty. This is the verbatim output the
// operator pasted in the bug report.
//
// Before the empty-result fix, `parseFRRBGPPaths` flipped this to
// PARSE_STATUS_PARSE_FAILED via the trailing `len(paths)==0 →
// Failed` guard — the WebUI/CLI then fell back to raw bytes for
// every zero-match lookup. This regression test pins the corrected
// semantics: zero-result is a successful parse with an empty
// payload.
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

// TestJSONParserFRREmptyResultsEnvelope locks in the bug fix for
// no-match FRR BGP lookups (community / large-community / aspath
// / route). Both chunks decode structurally — `Routes` is non-nil
// even when empty — so `decodedAny` flips true and the projector
// returns PARSE_STATUS_OK with an empty `paths` slice.
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

// FRR summary direct-AFI form with `peers: {}`. A freshly-built
// VRF with no peers configured yet — or a VRF whose peers haven't
// been added to the AFI in question — produces this shape.
// Before the fix, the trailing `len(out.Peers)==0 → Failed` guard
// in `parseFRRBGPSummary` flipped this to PARSE_FAILED.
const frrBGPSummaryEmptyPeersV4 = `{
  "routerId": "10.0.0.1",
  "as": 65000,
  "vrfId": 0,
  "vrfName": "default",
  "peers": { }
}`

// TestJSONParserFRRSummaryEmptyPeers pins the empty-peer-map
// semantics: PARSE_STATUS_OK with an empty `peers` slice. Router
// identifier + local-AS still propagate from the envelope header.
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

