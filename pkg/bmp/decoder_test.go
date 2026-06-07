package bmp

import (
	"encoding/binary"
	"testing"
)

// TestParseBMPCommonHeader verifies ParseBMPCommonHeader correctly parses 6-byte common headers.
func TestParseBMPCommonHeader(t *testing.T) {
	data := []byte{3, 0, 0, 0, 50, 0}
	version, length, msgType, err := ParseBMPCommonHeader(data)
	if err != nil {
		t.Fatalf("unexpected error parsing common header: %v", err)
	}
	if version != 3 || length != 50 || msgType != 0 {
		t.Errorf("unexpected values: version=%d, length=%d, type=%d", version, length, msgType)
	}

	_, _, _, err = ParseBMPCommonHeader(data[:3])
	if err == nil {
		t.Errorf("expected error for truncated header")
	}
}

// TestParsePerPeerHeader verifies ParsePerPeerHeader correctly decodes the metadata about a BGP peer.
func TestParsePerPeerHeader(t *testing.T) {
	data := make([]byte, bmpPerPeerHeaderLen)
	data[0] = 1
	data[1] = 0x00
	data[22] = 192
	data[23] = 0
	data[24] = 2
	data[25] = 1
	binary.BigEndian.PutUint32(data[26:30], 65001)

	h, err := ParsePerPeerHeader(data)
	if err != nil {
		t.Fatalf("unexpected error parsing per-peer header: %v", err)
	}
	if h.PeerType != 1 || h.PeerAddress != "192.0.2.1" || h.PeerAS != 65001 {
		t.Errorf("unexpected values: type=%d, addr=%s, AS=%d", h.PeerType, h.PeerAddress, h.PeerAS)
	}
	if h.IsIPv6() {
		t.Errorf("expected IsIPv6 to be false")
	}

	data[1] = 0x80
	copy(data[10:26], []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	h6, err := ParsePerPeerHeader(data)
	if err != nil {
		t.Fatalf("unexpected error parsing IPv6 per-peer header: %v", err)
	}
	if h6.PeerAddress != "2001:db8::1" {
		t.Errorf("unexpected IPv6 address: %s", h6.PeerAddress)
	}
	if !h6.IsIPv6() {
		t.Errorf("expected IsIPv6 to be true")
	}

	_, err = ParsePerPeerHeader(data[:10])
	if err == nil {
		t.Errorf("expected error for truncated peer header")
	}
}

// TestParseBGPUpdate verifies ParseBGPUpdate decodes BGP update messages without attributes.
func TestParseBGPUpdate(t *testing.T) {
	data := []byte{0, 0, 0, 0}
	up, err := ParseBGPUpdate(data)
	if err != nil {
		t.Fatalf("unexpected error parsing empty update: %v", err)
	}
	if len(up.AnnouncedPrefixes) != 0 || len(up.WithdrawnPrefixes) != 0 {
		t.Errorf("expected empty slices, got: %+v", up)
	}

	data2 := []byte{
		0, 0,
		0, 0,
		24, 192, 0, 2,
	}
	up2, err := ParseBGPUpdate(data2)
	if err != nil {
		t.Fatalf("unexpected error parsing simple announcements: %v", err)
	}
	if len(up2.AnnouncedPrefixes) != 1 || up2.AnnouncedPrefixes[0] != "192.0.2.0/24" {
		t.Errorf("unexpected announced prefixes: %v", up2.AnnouncedPrefixes)
	}

	_, err = ParseBGPUpdate([]byte{0})
	if err == nil {
		t.Errorf("expected error for truncated update")
	}
}

// TestHelperParsers verifies sub-parsers like AS path, communities, and MP_REACH/UNREACH NLRI.
func TestHelperParsers(t *testing.T) {
	asSeq4 := []byte{2, 2, 0, 0, 0, 100, 0, 0, 0, 101}
	path4 := parseASPath(asSeq4)
	if len(path4) != 2 || path4[0] != 100 || path4[1] != 101 {
		t.Errorf("unexpected 4-byte AS path: %v", path4)
	}

	asSeq2 := []byte{2, 2, 0, 100, 0, 101}
	path2 := parseASPath(asSeq2)
	if len(path2) != 2 || path2[0] != 100 || path2[1] != 101 {
		t.Errorf("unexpected 2-byte AS path: %v", path2)
	}

	commData := []byte{0, 100, 0, 200}
	comms := parseCommunities(commData)
	if len(comms) != 1 || comms[0] != "100:200" {
		t.Errorf("unexpected communities: %v", comms)
	}

	lcommData := []byte{0, 0, 0, 100, 0, 0, 0, 200, 0, 0, 1, 0}
	lcomms := parseLargeCommunities(lcommData)
	if len(lcomms) != 1 || lcomms[0] != "100:200:256" {
		t.Errorf("unexpected large communities: %v", lcomms)
	}

	nh := parseNextHop([]byte{192, 0, 2, 1})
	if nh != "192.0.2.1" {
		t.Errorf("unexpected next hop: %s", nh)
	}

	v6NextHop := []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	mpReach := append([]byte{0, 2, 1, 16}, v6NextHop...)
	mpReach = append(mpReach, 0)
	mpReach = append(mpReach, 64, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0)
	nh, prefixes, err := parseMPReachNLRI(mpReach)
	if err != nil {
		t.Fatalf("parseMPReachNLRI failed: %v", err)
	}
	if nh != "2001:db8::1" || len(prefixes) != 1 || prefixes[0] != "2001:db8::/64" {
		t.Errorf("unexpected MP_REACH: nh=%s, prefixes=%v", nh, prefixes)
	}

	mpUnreach := []byte{0, 2, 1}
	mpUnreach = append(mpUnreach, 64, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0)
	prefixes, err = parseMPUnreachNLRI(mpUnreach)
	if err != nil {
		t.Fatalf("parseMPUnreachNLRI failed: %v", err)
	}
	if len(prefixes) != 1 || prefixes[0] != "2001:db8::/64" {
		t.Errorf("unexpected MP_UNREACH: %v", prefixes)
	}
}

// TestIPContainsHelper verifies ipContains correctly reports containment of IP/prefix.
func TestIPContainsHelper(t *testing.T) {
	if !ipContains("192.0.2.0/24", "192.0.2.1") {
		t.Errorf("expected 192.0.2.0/24 to contain 192.0.2.1")
	}
	if ipContains("192.0.2.0/24", "198.51.100.1") {
		t.Errorf("expected 192.0.2.0/24 NOT to contain 198.51.100.1")
	}
	if ipContains("invalid", "1.1.1.1") {
		t.Errorf("invalid CIDR should not contain anything")
	}
}

// TestHelperParsersErrors verifies sub-parser error-handling for truncated or malformed inputs.
func TestHelperParsersErrors(t *testing.T) {
	_, _, err := parsePrefix([]byte{8}, false)
	if err == nil {
		t.Errorf("expected error for truncated prefix")
	}
	_, _, err = parsePrefix([]byte{}, false)
	if err == nil {
		t.Errorf("expected error for empty prefix")
	}

	asns := parseASPath([]byte{2, 1})
	if len(asns) != 0 {
		t.Errorf("expected 0 asns, got %v", asns)
	}
	asns = parseASPath([]byte{2})
	if len(asns) != 0 {
		t.Errorf("expected 0 asns, got %v", asns)
	}

	comms := parseCommunities([]byte{0, 100})
	if len(comms) != 0 {
		t.Errorf("expected 0 comms, got %v", comms)
	}

	lcomms := parseLargeCommunities([]byte{0, 0, 0, 100})
	if len(lcomms) != 0 {
		t.Errorf("expected 0 large comms, got %v", lcomms)
	}

	nh := parseNextHop([]byte{127, 0, 0})
	if nh != "" {
		t.Errorf("expected empty next hop, got %s", nh)
	}

	_, _, err = parseMPReachNLRI([]byte{0, 2, 1})
	if err == nil {
		t.Errorf("expected error for truncated MP_REACH")
	}
	_, _, err = parseMPReachNLRI([]byte{0, 2, 1, 16})
	if err == nil {
		t.Errorf("expected error for truncated next hop bytes")
	}

	_, err = parseMPUnreachNLRI([]byte{0, 2})
	if err == nil {
		t.Errorf("expected error for truncated MP_UNREACH")
	}
}

// TestParsePathAttributesErrors verifies path attributes parser error-handling.
func TestParsePathAttributesErrors(t *testing.T) {
	_, err := parsePathAttributes([]byte{0})
	if err == nil {
		t.Errorf("expected error for truncated header")
	}

	_, err = parsePathAttributes([]byte{0x10, 1})
	if err == nil {
		t.Errorf("expected error for truncated extended length")
	}

	_, err = parsePathAttributes([]byte{0x00, 1, 10, 0})
	if err == nil {
		t.Errorf("expected error for truncated value")
	}

	attrs, err := parsePathAttributes([]byte{0x10, 1, 0, 2, 9, 9})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attrs) != 1 || len(attrs[0].Value) != 2 {
		t.Errorf("unexpected attributes parsed: %+v", attrs)
	}
}

// TestParseBGPUpdateWithAttributes verifies ParseBGPUpdate parses AS path, community, and large community attributes.
func TestParseBGPUpdateWithAttributes(t *testing.T) {
	asAttr := []byte{0x40, 2, 6, 2, 1, 0, 0, 0, 100}
	commAttr := []byte{0x40, 8, 4, 0, 100, 0, 200}
	lcommAttr := []byte{0x40, 32, 12, 0, 0, 0, 100, 0, 0, 0, 200, 0, 0, 1, 0}

	attrs := append(asAttr, commAttr...)
	attrs = append(attrs, lcommAttr...)

	totalAttrsLen := len(attrs)

	data := make([]byte, 4+totalAttrsLen)
	binary.BigEndian.PutUint16(data[2:4], uint16(totalAttrsLen))
	copy(data[4:], attrs)

	update, err := ParseBGPUpdate(data)
	if err != nil {
		t.Fatalf("failed to parse BGP Update: %v", err)
	}

	if len(update.ASPath) != 1 || update.ASPath[0] != 100 {
		t.Errorf("expected ASPath [100], got %v", update.ASPath)
	}
	if len(update.Communities) != 1 || update.Communities[0] != "100:200" {
		t.Errorf("expected community 100:200, got %v", update.Communities)
	}
	if len(update.LargeCommunities) != 1 || update.LargeCommunities[0] != "100:200:256" {
		t.Errorf("expected large community 100:200:256, got %v", update.LargeCommunities)
	}
}

// TestParseBGPUpdateWithWithdrawnAndUnreach verifies ParseBGPUpdate parses standard withdrawn prefixes and MP_UNREACH attributes.
func TestParseBGPUpdateWithWithdrawnAndUnreach(t *testing.T) {
	withdrawn := []byte{24, 198, 51, 100}
	unreachAttr := []byte{0x80, 15, 12, 0, 2, 1, 64, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0}

	data := make([]byte, 4+len(withdrawn)+len(unreachAttr))
	binary.BigEndian.PutUint16(data[0:2], uint16(len(withdrawn)))
	copy(data[2:2+len(withdrawn)], withdrawn)

	offset := 2 + len(withdrawn)
	binary.BigEndian.PutUint16(data[offset:offset+2], uint16(len(unreachAttr)))
	copy(data[offset+2:], unreachAttr)

	update, err := ParseBGPUpdate(data)
	if err != nil {
		t.Fatalf("failed to parse BGP Update: %v", err)
	}

	if len(update.WithdrawnPrefixes) != 2 { // IPv4 withdrawal + IPv6 unreach withdrawal = 2 withdrawn prefixes
		t.Errorf("expected 2 withdrawn prefixes, got %d: %v", len(update.WithdrawnPrefixes), update.WithdrawnPrefixes)
	}
}

// TestParseBGPUpdateMalformed verifies ParseBGPUpdate's resilience to malformed path attributes or truncated NLRI.
func TestParseBGPUpdateMalformed(t *testing.T) {
	data := []byte{0, 0, 0, 10, 0, 1}
	_, _ = ParseBGPUpdate(data)

	data2 := []byte{0, 0, 0, 0, 24, 192}
	_, _ = ParseBGPUpdate(data2)
}
