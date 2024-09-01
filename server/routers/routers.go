package routers

import (
	"log"

	"github.com/AS203038/looking-glass/server/utils"
)

var (
	_routers = make(map[string]utils.Router)
)

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

func Get(name string) utils.Router {
	if _, ok := _routers[name]; !ok {
		return nil
	}
	return _routers[name]
}

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
