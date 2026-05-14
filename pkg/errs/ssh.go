package errs

import (
	"errors"
)

// SSH-execution sentinels. Returned by pkg/utils/ssh.go and propagated
// unchanged through the router and gRPC layers. They are intentionally
// coarse so that detailed failure context (hostnames, banner messages,
// remote stderr) stays in operator-facing server logs and never leaks
// to RPC clients.
var (
	// AuthFailed is returned when the SSH handshake completes but the
	// configured credentials are rejected by the router.
	AuthFailed = errors.New("authentication error")

	// ExecFailed is returned when a session is established but a
	// command exits non-zero, the remote shell terminates abnormally,
	// or stdout/stderr capture fails.
	ExecFailed = errors.New("execution error")

	// ConnectionFailed is returned when the underlying TCP dial or
	// SSH protocol negotiation fails before authentication is even
	// attempted.
	ConnectionFailed = errors.New("connection error")
)
