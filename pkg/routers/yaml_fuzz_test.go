package routers

import (
	"errors"
	"net/netip"
	"slices"
	"testing"

	"github.com/AS203038/looking-glass/pkg/errs"
	yaml "gopkg.in/yaml.v2"
)

// FuzzPeerRouteCommandsUnmarshalYAML verifies peerRouteCommands YAML decoding and command selection invariants.
func FuzzPeerRouteCommandsUnmarshalYAML(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("- show ip bgp peer {{.PeerIP}}"),
		[]byte("- show route protocol {{.PeerName}} all\n- show route export {{.PeerName}}"),
		[]byte("ipv4:\n  - show ip bgp neighbors {{.PeerIP}} received-routes\nipv6:\n  - show ipv6 bgp neighbors {{.PeerIP}} received-routes"),
		[]byte("any:\n  - cmd-any\nipv4:\n  - cmd-v4\nipv6:\n  - cmd-v6\nname:\n  - cmd-name-v4\n  - cmd-name-v6"),
		[]byte("[]"),
		[]byte("{}"),
		[]byte(""),
		[]byte("null"),
		[]byte("123"),
		[]byte("["),
		[]byte("ipv4: [\x00]"),
		[]byte("ipv4:\n  - \"show ip bgp; reload\""),
		[]byte("name: \"peer\\\"$(id)\""),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var p peerRouteCommands
		if err := yaml.Unmarshal(data, &p); err != nil {
			return
		}

		var asList []string
		if err := yaml.Unmarshal(data, &asList); err == nil && len(asList) > 0 {
			if !slices.Equal(p.Any, asList) {
				t.Errorf("list unmarshal mismatch: p.Any = %v, want %v", p.Any, asList)
			}
			if len(p.IPv4) != 0 || len(p.IPv6) != 0 || len(p.Name) != 0 {
				t.Errorf("list unmarshal populated mapping fields: %+v", p)
			}
		}

		allEmpty := len(p.Any) == 0 && len(p.IPv4) == 0 && len(p.IPv6) == 0 && len(p.Name) == 0
		if allEmpty {
			if cmds, err := p.commands("10.19.0.14", ""); cmds != nil || err != nil {
				t.Errorf("empty commands(IPv4) = (%v, %v), want (nil, nil)", cmds, err)
			}
			return
		}

		if len(p.IPv4) > 0 {
			cmds, err := p.commands("10.19.0.14", "GRP")
			if err != nil || !slices.Equal(cmds, p.IPv4) {
				t.Errorf("commands(IPv4) = (%v, %v), want %v", cmds, err, p.IPv4)
			}
		}
		if len(p.IPv6) > 0 {
			cmds, err := p.commands("2001:db8::1", "GRP")
			if err != nil || !slices.Equal(cmds, p.IPv6) {
				t.Errorf("commands(IPv6) = (%v, %v), want %v", cmds, err, p.IPv6)
			}
		}
		if len(p.Name) > 0 {
			cmds, err := p.commands("", "GRP")
			if err != nil || !slices.Equal(cmds, p.Name) {
				t.Errorf("commands(name-only) = (%v, %v), want %v", cmds, err, p.Name)
			}
		}
		_, _ = p.commands("", "")
	})
}

// FuzzPeerRouteCommandsSelect verifies peerRouteCommands.commands selection across IPv4, IPv6, name, and invalid inputs.
func FuzzPeerRouteCommandsSelect(f *testing.F) {
	seeds := []struct {
		peerIP   string
		peerName string
	}{
		{"10.19.0.14", ""},
		{"2001:db8::1", ""},
		{"fe80::1%et-0/0/1:0.0", ""},
		{"", "MY-GROUP"},
		{"10.19.0.14", "MY-GROUP"},
		{"", ""},
		{" ", "GRP"},
		{"::", ""},
		{"0.0.0.0", ""},
		{"10.0.0.1; reload", ""},
		{"fe80::1%eth0\nreload", ""},
		{"$(id)", "$(id)"},
		{"foo\x00bar", ""},
		{"' OR 1=1--", "' OR 1=1--"},
	}
	for _, s := range seeds {
		f.Add(s.peerIP, s.peerName)
	}

	ipOnly := peerRouteCommands{
		IPv4: []string{"v4"},
		IPv6: []string{"v6"},
	}
	combined := peerRouteCommands{
		IPv4: []string{"v4"},
		IPv6: []string{"v6"},
		Name: []string{"name-v4", "name-v6"},
	}

	f.Fuzz(func(t *testing.T, peerIP, peerName string) {
		cmdsIPOnly, errIPOnly := ipOnly.commands(peerIP, peerName)
		cmdsCombined, errCombined := combined.commands(peerIP, peerName)

		if peerIP != "" {
			addr, parseErr := netip.ParseAddr(peerIP)
			if parseErr != nil {
				if !errors.Is(errIPOnly, errs.IPInvalid) || !errors.Is(errCombined, errs.IPInvalid) {
					t.Errorf("invalid peerIP %q errors = (%v, %v), want IPInvalid", peerIP, errIPOnly, errCombined)
				}
				return
			}
			if addr.Is4() {
				if errIPOnly != nil || !slices.Equal(cmdsIPOnly, ipOnly.IPv4) ||
					errCombined != nil || !slices.Equal(cmdsCombined, combined.IPv4) {
					t.Errorf("IPv4 %q got (%v/%v, %v/%v), want IPv4 list", peerIP, cmdsIPOnly, errIPOnly, cmdsCombined, errCombined)
				}
				return
			}
			if addr.Is6() {
				if errIPOnly != nil || !slices.Equal(cmdsIPOnly, ipOnly.IPv6) ||
					errCombined != nil || !slices.Equal(cmdsCombined, combined.IPv6) {
					t.Errorf("IPv6 %q got (%v/%v, %v/%v), want IPv6 list", peerIP, cmdsIPOnly, errIPOnly, cmdsCombined, errCombined)
				}
				return
			}
		}

		if peerName != "" {
			if !errors.Is(errIPOnly, errs.PeerIPRequired) {
				t.Errorf("ipOnly name-only %q error = %v, want PeerIPRequired", peerName, errIPOnly)
			}
			if errCombined != nil || !slices.Equal(cmdsCombined, combined.Name) {
				t.Errorf("combined name-only %q = (%v, %v), want %v", peerName, cmdsCombined, errCombined, combined.Name)
			}
			return
		}

		if !errors.Is(errIPOnly, errs.PeerIPRequired) || !errors.Is(errCombined, errs.PeerIPRequired) {
			t.Errorf("empty operands errors = (%v, %v), want PeerIPRequired", errIPOnly, errCombined)
		}
	})
}
