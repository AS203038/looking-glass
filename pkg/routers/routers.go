// Package routers implements the vendor-template registry and the
// YAML-driven [utils.Router] type.
package routers

import (
	"log/slog"
	"os"

	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/utils"
)

// routersLog is the component-tagged logger for the router registry.
var routersLog = logging.Component("routers")

// Exit codes for fatal router-registry failures.
const (
	exitRouterNameEmpty       = 10
	exitRouterDuplicateRegist = 11
)

// _routers is the process-global template registry, populated by init().
var (
	_routers = make(map[string]utils.Router)
)

// register installs rt under name in the global registry. Exits the
// process on empty name or duplicate registration.
func register(name string, rt utils.Router) bool {
	if name == "" {
		routersLog.Error("router name cannot be empty")
		os.Exit(exitRouterNameEmpty)
	}
	if _, ok := _routers[name]; ok {
		routersLog.Error("router already registered", slog.String("router", name))
		os.Exit(exitRouterDuplicateRegist)
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
// Reachability is set by the background health-check loop in the grpc
// package; no probe is launched here.
func CreateRouterMap(cfg *utils.Config) utils.RouterMap {
	var rm utils.RouterMap
	skipped := 0
	for _, v := range cfg.Devices {
		rt := Get(v.Type)
		if rt == nil {
			routersLog.Error("router type not found; skipping device",
				slog.String("type", v.Type),
				slog.String("router", v.Name))
			skipped++
			continue
		}
		ri := &utils.RouterInstance{
			Config:      &v,
			Router:      Get(v.Type),
			HealthCheck: &utils.HealthCheck{},
		}
		routersLog.Debug("router bound",
			slog.String("router", v.Name),
			slog.String("type", v.Type),
			slog.String("host", v.Hostname))
		rm = append(rm, ri)
	}
	routersLog.Info("router catalogue built",
		slog.Int("devices", len(cfg.Devices)),
		slog.Int("registered", len(rm)),
		slog.Int("skipped", skipped))
	return rm
}
