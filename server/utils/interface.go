package utils

import "time"

type Router interface {
	Ping(*RouterConfig, *IPNet) ([]string, error)
	Traceroute(*RouterConfig, *IPNet) ([]string, error)
	BGPRoute(*RouterConfig, *IPNet) ([]string, error)
	BGPCommunity(*RouterConfig, string) ([]string, error)
	BGPASPath(*RouterConfig, string) ([]string, error)
}

type RouterInstance struct {
	Router      Router
	Config      *RouterConfig
	HealthCheck *HealthCheck
}

type HealthCheck struct {
	Checked time.Time
	Healthy bool
}

func (rt *RouterInstance) Healthcheck() error {
	_, err := SSHExec(rt.Config, []string{})
	rt.HealthCheck.Checked = time.Now()
	if err == nil {
		rt.HealthCheck.Healthy = true
	} else {
		rt.HealthCheck.Healthy = false
	}
	return err
}

func (rt *RouterInstance) Ping(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.Ping(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

func (rt *RouterInstance) Traceroute(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.Traceroute(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

func (rt *RouterInstance) BGPRoute(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.BGPRoute(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

func (rt *RouterInstance) BGPCommunity(param string) ([]string, error) {
	cmd, err := rt.Router.BGPCommunity(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

func (rt *RouterInstance) BGPASPath(param string) ([]string, error) {
	cmd, err := rt.Router.BGPASPath(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

type RouterMap []*RouterInstance

func (rm RouterMap) Get(name string) (*RouterInstance, bool) {
	for _, v := range rm {
		if v.Config.Name == name {
			return v, true
		}
	}
	return nil, false
}

func (rm RouterMap) GetByID(id int64) (*RouterInstance, bool) {
	id = id - 1
	if id < 0 || id >= int64(len(rm)) {
		return nil, false
	}
	return rm[id], true
}
