package routers

import (
	"fmt"

	"gitlab.as203038.net/AS203038/looking-glass/server/errs"
	"gitlab.as203038.net/AS203038/looking-glass/server/utils"
)

type FRRouting struct{}

var _ = register("frrouting", &FRRouting{})

func (rt *FRRouting) Ping(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	cmd := "ping -n %s -c 5 -I %s %s"
	if ip.IsIPv4() {
		return []string{fmt.Sprintf(cmd, "-4", cfg.Source4.IP, ip.IP)}, nil
	} else if ip.IsIPv6() {
		return []string{fmt.Sprintf(cmd, "-6", cfg.Source6.IP, ip.IP)}, nil
	}
	return nil, errs.FamilyInvalid
}

func (rt *FRRouting) Traceroute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	cmd := "traceroute %s -w 1 -q 1 -I --back --mtu -e -s %s %s"
	if ip.IsIPv4() {
		return []string{fmt.Sprintf(cmd, "-4", cfg.Source4.IP, ip.IP)}, nil
	} else if ip.IsIPv6() {
		return []string{fmt.Sprintf(cmd, "-6", cfg.Source6.IP, ip.IP)}, nil
	}
	return nil, errs.FamilyInvalid
}

func (rt *FRRouting) BGPRoute(cfg *utils.RouterConfig, ip *utils.IPNet) ([]string, error) {
	cmd := "vtysh -c 'show bgp vrf %s %s unicast %s'"
	return []string{fmt.Sprintf(cmd, cfg.VRF, ip.Family, ip.IP)}, nil
}

func (rt *FRRouting) BGPCommunity(cfg *utils.RouterConfig, community string) ([]string, error) {
	return []string{
		fmt.Sprintf("vtysh -c 'show bgp vrf %s ipv4 unicast community %s'", cfg.VRF, community),
		fmt.Sprintf("vtysh -c 'show bgp vrf %s ipv6 unicast community %s'", cfg.VRF, community),
	}, nil
}

func (rt *FRRouting) BGPASPath(cfg *utils.RouterConfig, aspath string) ([]string, error) {
	return []string{
		fmt.Sprintf("vtysh -c 'show bgp vrf %s ipv4 unicast regexp %s'", cfg.VRF, aspath),
		fmt.Sprintf("vtysh -c 'show bgp vrf %s ipv6 unicast regexp %s'", cfg.VRF, aspath),
	}, nil
}
