package utils

import "time"

// Router is the contract every vendor template implements: it returns
// the command sequence for each supported diagnostic operation.
type Router interface {
	// Ping returns the command sequence that probes reachability.
	Ping(*RouterConfig, *IPNet) ([]string, error)
	// Traceroute returns the command sequence that records hops to the target.
	Traceroute(*RouterConfig, *IPNet) ([]string, error)
	// BGPSummary returns the command sequence for the BGP neighbour summary.
	BGPSummary(*RouterConfig) ([]string, error)
	// BGPRoute returns the command sequence for a BGP-path lookup by prefix.
	BGPRoute(*RouterConfig, *IPNet) ([]string, error)
	// BGPCommunity returns the command sequence for a standard-community lookup.
	BGPCommunity(*RouterConfig, string) ([]string, error)
	// BGPLargeCommunity returns the command sequence for an RFC 8092 large-community lookup.
	BGPLargeCommunity(*RouterConfig, string) ([]string, error)
	// BGPASPath returns the command sequence for an AS-path regex lookup.
	BGPASPath(*RouterConfig, string) ([]string, error)
	// BGPPeerRoutes returns the command sequence for peer session route lookups.
	BGPPeerRoutes(*RouterConfig, string, string, string) ([]string, error)
}

// RouterInstance binds a [Router] implementation to its per-device
// [RouterConfig] and a shared [HealthCheck] record.
type RouterInstance struct {
	// Router is the vendor-specific template for this device.
	Router Router
	// Config holds the connection parameters for this device.
	Config *RouterConfig
	// HealthCheck stores the latest reachability state.
	HealthCheck *HealthCheck
}

// HealthCheck records the outcome of the most recent reachability probe.
type HealthCheck struct {
	// Checked is the timestamp of the last completed probe attempt.
	Checked time.Time
	// Healthy reports whether the last probe completed without an SSH error.
	Healthy bool
}

// Healthcheck runs a no-op SSH session against the router and updates the
// [HealthCheck] record with the timestamp and outcome.
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

// Ping renders and executes the ping command sequence for this router.
func (rt *RouterInstance) Ping(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.Ping(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// Traceroute renders and executes the traceroute command sequence.
func (rt *RouterInstance) Traceroute(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.Traceroute(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPSummary renders and executes the neighbour-summary command sequence.
func (rt *RouterInstance) BGPSummary() ([]string, error) {
	cmd, err := rt.Router.BGPSummary(rt.Config)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPRoute renders and executes the BGP-route lookup command sequence.
func (rt *RouterInstance) BGPRoute(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.BGPRoute(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPCommunity renders and executes the standard-community lookup command sequence.
func (rt *RouterInstance) BGPCommunity(param string) ([]string, error) {
	cmd, err := rt.Router.BGPCommunity(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPLargeCommunity renders and executes the large-community lookup command sequence.
func (rt *RouterInstance) BGPLargeCommunity(param string) ([]string, error) {
	cmd, err := rt.Router.BGPLargeCommunity(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPASPath renders and executes the AS-path regex lookup command sequence.
func (rt *RouterInstance) BGPASPath(param string) ([]string, error) {
	cmd, err := rt.Router.BGPASPath(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPPeerRoutes renders and executes the peer session route lookup command sequence.
func (rt *RouterInstance) BGPPeerRoutes(peerIP, peerName, queryType string) ([]string, error) {
	cmd, err := rt.Router.BGPPeerRoutes(rt.Config, peerIP, peerName, queryType)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// RouterMap is the in-memory catalogue of every configured router.
type RouterMap []*RouterInstance

// Get returns the first router whose configured name matches name exactly.
func (rm RouterMap) Get(name string) (*RouterInstance, bool) {
	for _, v := range rm {
		if v.Config.Name == name {
			return v, true
		}
	}
	return nil, false
}

// GetByID returns the router whose 1-based public identifier is id.
func (rm RouterMap) GetByID(id int64) (*RouterInstance, bool) {
	id = id - 1
	if id < 0 || id >= int64(len(rm)) {
		return nil, false
	}
	return rm[id], true
}
