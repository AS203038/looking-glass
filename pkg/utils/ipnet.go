package utils

import (
	"net"
	"strings"

	"github.com/AS203038/looking-glass/pkg/errs"
)

type IPFamily string

const (
	IPv4 IPFamily = "ipv4"
	IPv6 IPFamily = "ipv6"
)

type IPNet struct {
	IP     string
	CIDR   string
	Family IPFamily
}

func (ip *IPNet) IsIPv4() bool {
	return ip.Family == IPv4
}

func (ip *IPNet) IsIPv6() bool {
	return ip.Family == IPv6
}

func (ip *IPNet) FamilyString() string {
	return string(ip.Family)
}

func (ip *IPNet) String() string {
	return ip.IP + "/" + ip.CIDR
}

func (ip *IPNet) ToIPNet() *net.IPNet {
	_, ipnet, _ := net.ParseCIDR(ip.String())
	return ipnet
}

func (ip *IPNet) ToIP() net.IP {
	return net.ParseIP(ip.IP)
}

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
