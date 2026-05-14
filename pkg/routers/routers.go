// Package routers implements the vendor-template registry and the
// YAML-driven [utils.Router] type that backs every shipped template.
//
// At process start the [init] function in yaml.go walks both the
// optional ROUTER_DIR directory and the bundled `*.yml` files,
// registering each named template in the unexported `_routers`
// map. Operator configuration (see [utils.RouterConfig.Type]) then
// references these names so that adding support for a new vendor
// is a pure-data exercise — no Go code required.
package routers

import (
	"log"

	"github.com/AS203038/looking-glass/pkg/utils"
)

// _routers is the process-global template registry, populated by
// init() in yaml.go. It is read-only after start-up; concurrent
// reads from request handlers are safe without further
// synchronisation.
var (
	_routers = make(map[string]utils.Router)
)

// register installs rt under name in the global registry. It is
// invoked from init() while loading template files.
//
// The function panics on empty names or duplicate registrations:
// both indicate a packaging bug that the operator cannot recover
// from at runtime, so failing fast is preferred over surfacing
// confusing behaviour later.
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

// Get returns the registered template for the supplied type name
// or nil when no template by that name has been registered.
func Get(name string) utils.Router {
	if _, ok := _routers[name]; !ok {
		return nil
	}
	return _routers[name]
}

// CreateRouterMap materialises the operator's device list into a
// [utils.RouterMap]. For each [utils.RouterConfig]:
//
//   - The referenced template type is resolved via [Get]. Unknown
//     types are logged and the device is skipped (the server keeps
//     starting so a typo in one device does not nuke the whole
//     instance).
//   - A [utils.RouterInstance] is constructed binding the template
//     to the device config and a fresh [utils.HealthCheck].
//   - A background goroutine is started that runs an initial
//     reachability probe; further probes are scheduled by the gRPC
//     layer's periodic ticker (see pkg/http/grpc/grpc.go).
//
// The returned [utils.RouterMap] preserves the order of cfg.Devices;
// element index plus one is the router's stable, public ID.
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
