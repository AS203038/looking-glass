// Package routers implements the vendor-template registry and the
// YAML-driven [utils.Router] type.
package routers

import (
	"log"

	"github.com/AS203038/looking-glass/pkg/utils"
)

// _routers is the process-global template registry, populated by init().
var (
	_routers = make(map[string]utils.Router)
)

// register installs rt under name in the global registry. Panics on an
// empty name or a duplicate registration.
func register(name string, rt utils.Router) bool {
	if name == "" {
		log.Panicln("ERROR: Router name cannot be empty")
	}
	if _, ok := _routers[name]; ok {
		log.Panicf("WARNING: Router %s already registered", name)
	}
	_routers[name] = rt
	return true
}

// Get returns the registered template for name, or nil when none is registered.
func Get(name string) utils.Router {
	if _, ok := _routers[name]; !ok {
		return nil
	}
	return _routers[name]
}

// CreateRouterMap materialises cfg.Devices into a [utils.RouterMap].
// Devices referencing an unknown template type are logged and skipped.
// Each instance is started with an initial reachability probe in a
// background goroutine.
func CreateRouterMap(cfg *utils.Config) utils.RouterMap {
	var rm utils.RouterMap
	for _, v := range cfg.Devices {
		rt := Get(v.Type)
		if rt == nil {
			log.Printf("ERROR: Router Type %s not found (%s)\n", v.Type, v.Name)
			continue
		}
		ri := &utils.RouterInstance{
			Config:      &v,
			Router:      Get(v.Type),
			HealthCheck: &utils.HealthCheck{},
		}
		go ri.Healthcheck()
		rm = append(rm, ri)
	}
	return rm
}
