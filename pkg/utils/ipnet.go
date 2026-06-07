package utils

import (
	"net"
	"strings"

	"github.com/AS203038/looking-glass/pkg/errs"
)

// IPFamily enumerates the IP address families supported by the router templates.
type IPFamily string

// Supported [IPFamily] values.
const (
	// IPv4 marks an IPv4 address or prefix.
	IPv4 IPFamily = "ipv4"
	// IPv6 marks an IPv6 address or prefix.
	IPv6 IPFamily = "ipv6"
)

// IPNet is the validated, parsed representation of an IP address or CIDR prefix.
type IPNet struct {
	// IP is the host portion in canonical (un-bracketed) form.
	IP string
	// CIDR is the prefix-length portion (without the leading "/").
	CIDR string
	// Family is the detected address family.
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

// FamilyString returns the family as its string form ("ipv4" or "ipv6").
func (ip *IPNet) FamilyString() string {
	return string(ip.Family)
}

// String renders the address in canonical "IP/CIDR" form.
func (ip *IPNet) String() string {
	return ip.IP + "/" + ip.CIDR
}

// ToIPNet returns the address as a standard-library [*net.IPNet].
func (ip *IPNet) ToIPNet() *net.IPNet {
	_, ipnet, _ := net.ParseCIDR(ip.String())
	return ipnet
}

// ToIP returns the host portion as a standard-library [net.IP].
func (ip *IPNet) ToIP() net.IP {
	return net.ParseIP(ip.IP)
}

// UnmarshalYAML decodes an [IPNet] from a YAML scalar via [NewIPNET].
func (ip *IPNet) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var tmp string
	if err := unmarshal(&tmp); err != nil {
		return err
	}
	i, err := NewIPNET(tmp)
	if err != nil {
		return err
	}
	ip.IP = i.IP
	ip.CIDR = i.CIDR
	ip.Family = i.Family
	return nil
}

// NewIPNET parses a textual IP address or CIDR prefix into an [IPNet].
// Returns [errs.IPInvalid] or [errs.NetInvalid] for malformed input.
func NewIPNET(ip string) (*IPNet, error) {
	var ret = &IPNet{}
	ret.IP = ip
	hasSlash := strings.Contains(ip, "/")
	if hasSlash {
		if _, _, err := net.ParseCIDR(ip); err != nil {
			return nil, errs.NetInvalid
		}
		ret.CIDR = strings.Split(ip, "/")[1]
		ret.IP = strings.Split(ip, "/")[0]
	}
	if net.ParseIP(ret.IP) == nil {
		return nil, errs.IPInvalid
	}
	if strings.Contains(ret.IP, ":") {
		ret.Family = IPv6
	} else {
		ret.Family = IPv4
	}
	if !hasSlash {
		if ret.Family == IPv4 {
			ret.CIDR = "32"
		} else {
			ret.CIDR = "128"
		}
	}
	return ret, nil
}

// NewIPNetFromProtobuf parses target as a literal IP/CIDR or, failing that,
// resolves it as a hostname via DNS and returns the first address.
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
