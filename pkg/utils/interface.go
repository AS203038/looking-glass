package utils

import "time"

// Router is the contract every vendor template implements.
//
// A Router translates a high-level diagnostic request (ping a host,
// trace a route, look up a BGP prefix, …) into one or more
// vendor-specific shell commands. It never executes those commands
// itself; execution is delegated to [SSHExec] via [RouterInstance]
// so a single connection lifecycle and credential-redaction policy
// apply uniformly across all vendors.
//
// Implementations are expected to be pure: each method must derive
// its output solely from its arguments (RouterConfig + the operand)
// and must not retain per-call state. The YAML-driven implementation
// in pkg/routers is the canonical reference.
type Router interface {
	// Ping returns the command sequence that probes reachability to
	// the supplied address from the router.
	Ping(*RouterConfig, *IPNet) ([]string, error)
	// Traceroute returns the command sequence that records each hop
	// between the router and the supplied address.
	Traceroute(*RouterConfig, *IPNet) ([]string, error)
	// BGPRoute returns the command sequence that prints the best
	// (and, where supported, alternate) BGP paths to the supplied
	// prefix or address.
	BGPRoute(*RouterConfig, *IPNet) ([]string, error)
	// BGPCommunity returns the command sequence that lists every
	// route tagged with the supplied standard BGP community.
	BGPCommunity(*RouterConfig, string) ([]string, error)
	// BGPLargeCommunity returns the command sequence that lists
	// every route tagged with the supplied RFC 8092 large community.
	BGPLargeCommunity(*RouterConfig, string) ([]string, error)
	// BGPASPath returns the command sequence that lists every route
	// whose AS-path matches the supplied regular expression.
	BGPASPath(*RouterConfig, string) ([]string, error)
}

// RouterInstance binds a [Router] implementation to its per-device
// [RouterConfig] and a shared [HealthCheck] record. It is the unit
// of work the gRPC handlers operate on: one instance per configured
// device, looked up by ID via [RouterMap.GetByID].
type RouterInstance struct {
	// Router is the vendor-specific template that knows how to
	// build commands for this device's platform.
	Router Router
	// Config holds the operator-supplied connection parameters and
	// source addresses for this device.
	Config *RouterConfig
	// HealthCheck stores the latest reachability state and is
	// updated by the background goroutine started in
	// pkg/routers.CreateRouterMap.
	HealthCheck *HealthCheck
}

// HealthCheck records the outcome of the most recent reachability
// probe against a router. It is read by the gRPC service to surface
// device status to the UI and is updated in-place by the per-router
// background probe goroutine; concurrent reads are safe under Go's
// memory model because the consumer only reads scalars and never
// observes torn struct values.
type HealthCheck struct {
	// Checked is the timestamp of the last completed probe attempt,
	// regardless of outcome.
	Checked time.Time
	// Healthy reports whether the last probe completed without an
	// SSH error.
	Healthy bool
}

// Healthcheck runs a no-op SSH session against the router to verify
// that the network path, authentication and shell are all working.
// The HealthCheck struct is updated in-place with the timestamp and
// outcome regardless of error; the underlying error is also returned
// so callers can log diagnostic detail.
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

// Ping renders the ping command sequence for this router and
// executes it over SSH. Returns one stdout string per command in the
// order rendered.
func (rt *RouterInstance) Ping(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.Ping(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// Traceroute renders the traceroute command sequence for this router
// and executes it over SSH.
func (rt *RouterInstance) Traceroute(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.Traceroute(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPRoute renders the BGP-route lookup command sequence for this
// router and executes it over SSH.
func (rt *RouterInstance) BGPRoute(param *IPNet) ([]string, error) {
	cmd, err := rt.Router.BGPRoute(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPCommunity renders the standard-community lookup command
// sequence for this router and executes it over SSH.
func (rt *RouterInstance) BGPCommunity(param string) ([]string, error) {
	cmd, err := rt.Router.BGPCommunity(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPLargeCommunity renders the RFC 8092 large-community lookup
// command sequence for this router and executes it over SSH.
func (rt *RouterInstance) BGPLargeCommunity(param string) ([]string, error) {
	cmd, err := rt.Router.BGPLargeCommunity(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// BGPASPath renders the AS-path regex lookup command sequence for
// this router and executes it over SSH. The regex must already have
// been validated via [SanitizeASPath].
func (rt *RouterInstance) BGPASPath(param string) ([]string, error) {
	cmd, err := rt.Router.BGPASPath(rt.Config, param)
	if err != nil {
		return nil, err
	}
	return SSHExec(rt.Config, cmd)
}

// RouterMap is the in-memory catalogue of every configured router.
// Slice order is significant: each element's index plus one is its
// public, stable identifier (returned to clients by the gRPC service
// and used by [RouterMap.GetByID]). Treat the slice as immutable
// after process start; concurrent reads are safe.
type RouterMap []*RouterInstance

// Get returns the first router whose configured name matches name
// exactly. The boolean reports whether a match was found.
func (rm RouterMap) Get(name string) (*RouterInstance, bool) {
	for _, v := range rm {
		if v.Config.Name == name {
			return v, true
		}
	}
	return nil, false
}

// GetByID returns the router whose public identifier is id. IDs are
// 1-based to keep the protobuf wire representation friendly to
// humans (no router has ID 0). Out-of-range IDs return (nil, false).
func (rm RouterMap) GetByID(id int64) (*RouterInstance, bool) {
	id = id - 1
	if id < 0 || id >= int64(len(rm)) {
		return nil, false
	}
	return rm[id], true
}
