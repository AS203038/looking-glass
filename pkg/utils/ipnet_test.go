package utils

import (
	"errors"
	"net"
	"testing"

	"github.com/AS203038/looking-glass/pkg/errs"
	yaml "gopkg.in/yaml.v2"
)

// TestNewIPNETValid verifies that valid IP and CIDR prefix strings are parsed correctly.
func TestNewIPNETValid(t *testing.T) {
	cases := []struct {
		input  string
		ip     string
		cidr   string
		family IPFamily
	}{
		{"1.1.1.1", "1.1.1.1", "32", IPv4},
		{"192.168.0.1/24", "192.168.0.1", "24", IPv4},
		{"2001:db8::1", "2001:db8::1", "128", IPv6},
		{"2001:db8::1/64", "2001:db8::1", "64", IPv6},
		{"0.0.0.0", "0.0.0.0", "32", IPv4},
		{"::", "::", "128", IPv6},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := NewIPNET(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got.IP != tc.ip {
				t.Errorf("IP = %q, want %q", got.IP, tc.ip)
			}
			if got.CIDR != tc.cidr {
				t.Errorf("CIDR = %q, want %q", got.CIDR, tc.cidr)
			}
			if got.Family != tc.family {
				t.Errorf("Family = %q, want %q", got.Family, tc.family)
			}
		})
	}
}

// TestNewIPNETInvalid verifies that invalid inputs return the correct sentinel errors.
func TestNewIPNETInvalid(t *testing.T) {
	cases := []struct {
		input       string
		expectedErr error
	}{
		{"", errs.IPInvalid},
		{"invalid", errs.IPInvalid},
		{"999.999.999.999", errs.IPInvalid},
		{"1.1.1.1/abc", errs.NetInvalid},
		{"1.1.1.1/33", errs.NetInvalid},
		{"2001:db8::1/129", errs.NetInvalid},
		{"1.1.1.1/", errs.NetInvalid},
		{"/128", errs.NetInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			_, err := NewIPNET(tc.input)
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("got error %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

// TestIPNetHelpers verifies the helper methods of IPNet.
func TestIPNetHelpers(t *testing.T) {
	ip4, _ := NewIPNET("192.168.1.1/24")
	if !ip4.IsIPv4() {
		t.Errorf("expected IsIPv4() to be true")
	}
	if ip4.IsIPv6() {
		t.Errorf("expected IsIPv6() to be false")
	}
	if ip4.FamilyString() != "ipv4" {
		t.Errorf("FamilyString() = %q, want ipv4", ip4.FamilyString())
	}
	if ip4.String() != "192.168.1.1/24" {
		t.Errorf("String() = %q, want 192.168.1.1/24", ip4.String())
	}

	ipnet := ip4.ToIPNet()
	if ipnet == nil || ipnet.IP.String() != "192.168.1.0" {
		t.Errorf("ToIPNet() failed: %v", ipnet)
	}

	ipObj := ip4.ToIP()
	if ipObj == nil || !ipObj.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("ToIP() failed: %v", ipObj)
	}

	ip6, _ := NewIPNET("2001:db8::1/64")
	if !ip6.IsIPv6() {
		t.Errorf("expected IsIPv6() to be true")
	}
	if ip6.IsIPv4() {
		t.Errorf("expected IsIPv4() to be false")
	}
}

// TestNewIPNetFromProtobuf verifies protobuf conversion and hostname resolution.
func TestNewIPNetFromProtobuf(t *testing.T) {
	// 1. Literal IP
	ip, err := NewIPNetFromProtobuf("1.1.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip.IP != "1.1.1.1" {
		t.Errorf("got IP %q, want 1.1.1.1", ip.IP)
	}

	// 2. DNS Resolution of localhost
	ipDNS, err := NewIPNetFromProtobuf("localhost")
	if err != nil {
		t.Fatalf("unexpected error resolving localhost: %v", err)
	}
	if ipDNS.IP != "127.0.0.1" && ipDNS.IP != "::1" {
		t.Errorf("unexpected resolved IP for localhost: %q", ipDNS.IP)
	}

	// 3. Invalid Hostname / Empty Input
	_, err = NewIPNetFromProtobuf("")
	if !errors.Is(err, errs.IPInvalid) {
		t.Errorf("expected IPInvalid, got %v", err)
	}

	_, err = NewIPNetFromProtobuf("non-existent-hostname-xyz.invalid")
	if err == nil {
		t.Errorf("expected error for non-existent hostname")
	}
}

// TestIPNetUnmarshalYAML verifies unmarshaling an IPNet from a YAML document.
func TestIPNetUnmarshalYAML(t *testing.T) {
	type Config struct {
		IP *IPNet `yaml:"ip"`
	}

	yamlDoc := `ip: "192.168.1.1/24"`
	var cfg Config
	err := yaml.Unmarshal([]byte(yamlDoc), &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	if cfg.IP == nil {
		t.Fatalf("unmarshaled IP is nil")
	}

	if cfg.IP.IP != "192.168.1.1" || cfg.IP.CIDR != "24" || cfg.IP.Family != IPv4 {
		t.Errorf("unmarshaled IP mismatch: got %+v", cfg.IP)
	}

	// Test unmarshaling error on invalid IP
	invalidYamlDoc := `ip: "invalid-ip-net"`
	var cfgInvalid Config
	err = yaml.Unmarshal([]byte(invalidYamlDoc), &cfgInvalid)
	if err == nil {
		t.Errorf("expected error unmarshaling invalid IPNet")
	}
}
