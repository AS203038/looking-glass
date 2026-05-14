package utils

import (
	"net"
	"strings"

	"github.com/AS203038/looking-glass/pkg/errs"
)

// IPFamily enumerates the IP address families supported by the
// router templates. The string values are deliberately lowercase
// because they are interpolated verbatim into vendor commands
// (e.g. Cisco IOS-XE expects "ipv4" / "ipv6" as keywords).
type IPFamily string

// Supported [IPFamily] values.
const (
	// IPv4 marks an IPv4 address or prefix.
	IPv4 IPFamily = "ipv4"
	// IPv6 marks an IPv6 address or prefix.
	IPv6 IPFamily = "ipv6"
)

// IPNet is the validated, parsed representation of an IP address or
// CIDR prefix accepted by the gRPC service. It is the only type ever
// interpolated into router templates so that template authors can
// safely call methods like [IPNet.IsIPv4] without re-validating user
// input themselves.
type IPNet struct {
	// IP is the host portion in canonical (un-bracketed) form.
	IP string
	// CIDR is the prefix-length portion (without the leading "/").
	// Defaults to "32" for v4 and "128" for v6 when omitted from
	// the input.
	CIDR string
	// Family is the detected address family. Always populated for
	// values returned by [NewIPNET] / [NewIPNetFromProtobuf].
	Family IPFamily
}

// IsIPv4 reports whether the address is in the IPv4 family.
func (ip *IPNet) IsIPv4() bool {
	return ip.Family == IPv4
}

// IsIPv6 reports whether the address is in the IPv6 family.
func (ip *IPNet) IsIPv6() bool {
	return ip.Family == IPv6
}

// FamilyString returns the family as its string form ("ipv4" or
// "ipv6"). Convenience wrapper for templates which cannot call
// methods that return non-string types.
func (ip *IPNet) FamilyString() string {
	return string(ip.Family)
}

// String renders the address in canonical "IP/CIDR" form.
func (ip *IPNet) String() string {
	return ip.IP + "/" + ip.CIDR
}

// ToIPNet returns the address as a standard-library [*net.IPNet].
// The error from net.ParseCIDR is intentionally discarded because
// String always emits a valid CIDR for any IPNet produced by the
// constructors in this package.
func (ip *IPNet) ToIPNet() *net.IPNet {
	_, ipnet, _ := net.ParseCIDR(ip.String())
	return ipnet
}

// ToIP returns the host portion as a standard-library [net.IP].
func (ip *IPNet) ToIP() net.IP {
	return net.ParseIP(ip.IP)
}

// UnmarshalYAML decodes an [IPNet] from a YAML scalar via
// [NewIPNET], so that the same validation rules apply whether the
// address arrives from config.yaml or from an RPC request.
func (ip *IPNet) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var tmp string
	if err := unmarshal(&tmp); err != nil {
		return err
	}
	i, err := NewIPNET(tmp)
	ip.IP = i.IP
	ip.CIDR = i.CIDR
	ip.Family = i.Family
	return err
}

// NewIPNET parses a textual IP address or CIDR prefix into an
// [IPNet]. The function accepts both bare addresses ("192.0.2.1",
// "2001:db8::1") and prefixes ("192.0.2.0/24"); the prefix length
// defaults to /32 or /128 when omitted.
//
// Returns [errs.IPInvalid] or [errs.NetInvalid] for malformed input.
func NewIPNET(ip string) (*IPNet, error) {
	var ret = &IPNet{}
	ret.IP = ip
	if strings.Contains(ip, "/") {
		if _, _, err := net.ParseCIDR(ip); err != nil {
			return nil, errs.NetInvalid
		}
		ret.CIDR = strings.Split(ip, "/")[1]
		ret.IP = strings.Split(ip, "/")[0]
	} else {
		if ret.Family == IPv4 {
			ret.CIDR = "32"
		} else {
			ret.CIDR = "128"
		}
	}
	if net.ParseIP(ret.IP) == nil {
		return nil, errs.IPInvalid
	}
	if strings.Contains(ret.IP, ":") {
		ret.Family = IPv6
	} else {
		ret.Family = IPv4
	}
	return ret, nil
}

// NewIPNetFromProtobuf is the entry point used by the gRPC service.
// It accepts either a literal IP/CIDR (handled by [NewIPNET]) or a
// hostname, in which case it performs a DNS lookup and uses the
// first returned address.
//
// Empty input yields [errs.IPInvalid] without touching DNS so that
// clients sending a missing field do not trigger resolver traffic.
func NewIPNetFromProtobuf(target string) (*IPNet, error) {
	if len(target) == 0 {
		return nil, errs.IPInvalid
	}
	if ret, err := NewIPNET(target); err == nil {
		return ret, nil
	}
	tmp, err := net.LookupHost(target)
	if err != nil {
		return nil, err
	}
	if len(tmp) != 0 {
		return NewIPNET(tmp[0])
	}
	return nil, errs.IPInvalid
}
