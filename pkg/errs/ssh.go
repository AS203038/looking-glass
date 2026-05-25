package errs

import (
	"errors"
)

// AuthFailed is returned when SSH credentials are rejected by the remote.
var AuthFailed = errors.New("authentication error")

// ExecFailed is returned when a remote command or session fails after the SSH connection is established.
var ExecFailed = errors.New("execution error")

// ConnectionFailed is returned when the TCP dial or SSH negotiation fails before authentication.
var ConnectionFailed = errors.New("connection error")

// PoolExhausted is returned when the SSH connection pool is fully utilized and has no available slots.
var PoolExhausted = errors.New("ssh pool exhausted")
