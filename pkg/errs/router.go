package errs

import (
	"errors"
)

// Router-orchestration sentinels. Returned by the router registry
// (pkg/routers) and the gRPC service layer when a request cannot be
// dispatched to a router instance.
var (
	// UnknownRouter is returned when the router type referenced by a
	// device configuration does not correspond to any registered
	// router template.
	UnknownRouter = errors.New("router unknown")

	// RouterUnavailable is returned when the requested router exists
	// but its background health check reports it as unreachable. It
	// is also surfaced when a router lookup by ID misses entirely.
	RouterUnavailable = errors.New("router unavailable")

	// OperationUnknown is returned when a router template does not
	// define a command sequence for the requested operation (e.g. a
	// vendor template that lacks a `bgp.aspath` block).
	OperationUnknown = errors.New("operation unknown")
)
