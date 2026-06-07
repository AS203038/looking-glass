package routers

import (
	"testing"

	"github.com/AS203038/looking-glass/pkg/utils"
)

// TestGetDefaultRouters verifies that default vendor templates are registered automatically.
func TestGetDefaultRouters(t *testing.T) {
	vendors := []string{
		"arista_eos",
		"cisco_ios",
		"frrouting",
		"juniper_junos",
		"mikrotik_routeros",
		"nokia_sros",
	}

	for _, name := range vendors {
		t.Run(name, func(t *testing.T) {
			rt := Get(name)
			if rt == nil {
				t.Errorf("router type %q is not registered in the global catalog", name)
			}
		})
	}

	nonExistent := Get("non_existent")
	if nonExistent != nil {
		t.Errorf("Get returned non-nil for non-existent router type")
	}
}

// TestCreateRouterMap verifies that a RouterMap is correctly constructed from Config.
func TestCreateRouterMap(t *testing.T) {
	cfg := &utils.Config{
		Devices: []utils.RouterConfig{
			{
				Name:     "r1",
				Type:     "frrouting",
				Hostname: "10.0.0.1",
			},
			{
				Name:     "r2",
				Type:     "arista_eos",
				Hostname: "10.0.0.2",
			},
			{
				Name:     "r3",
				Type:     "unknown_vendor",
				Hostname: "10.0.0.3",
			},
		},
	}

	rm := CreateRouterMap(cfg)

	// r3 is skipped because its type is unknown, so registered length should be 2.
	if len(rm) != 2 {
		t.Fatalf("len(rm) = %d, want 2 (r3 should be skipped)", len(rm))
	}

	if rm[0].Config.Name != "r1" || rm[0].Config.Type != "frrouting" {
		t.Errorf("first router instance mismatch: got %+v", rm[0].Config)
	}

	if rm[1].Config.Name != "r2" || rm[1].Config.Type != "arista_eos" {
		t.Errorf("second router instance mismatch: got %+v", rm[1].Config)
	}
}
