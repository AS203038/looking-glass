package parse

import (
	"testing"
)

// FuzzParseLinuxPing verifies that parseLinuxPing never panics under any string input.
func FuzzParseLinuxPing(f *testing.F) {
	seeds := []string{
		linuxPingIPv4Sample,
		linuxPingLossSample,
		linuxPingIPv6Sample,
		"invalid random text",
		"",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_ = parseLinuxPing([]byte(input))
	})
}

// FuzzParseLinuxTraceroute verifies that parseLinuxTraceroute never panics under any string input.
func FuzzParseLinuxTraceroute(f *testing.F) {
	seeds := []string{
		linuxTracerouteSample,
		"invalid random text",
		"",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_ = parseLinuxTraceroute([]byte(input))
	})
}
