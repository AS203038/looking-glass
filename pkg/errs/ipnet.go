// Package errs defines the sentinel errors surfaced by the Looking Glass
// backend.
//
// Errors in this package are intentionally coarse: they describe a *class*
// of failure (invalid input, router unreachable, SSH auth failure, …)
// rather than a specific message. Callers compare against them with
// [errors.Is] and translate them into ConnectRPC status codes at the
// gRPC boundary. Detailed, sensitive context (hostnames, credentials,
// raw command output) is logged server-side instead of being returned
// to the client.
package errs

import (
	"errors"
)

// IP- and network-parsing sentinels. Returned by helpers in
// pkg/utils/ipnet.go when an operand fails validation before any
// SSH work is attempted.
var (
	// IPInvalid is returned when an input string cannot be parsed as
	// either an IPv4 or IPv6 address.
	IPInvalid = errors.New("invalid IP")

	// NetInvalid is returned when an input string cannot be parsed as
	// a CIDR network, or when the resulting prefix is malformed
	// (e.g. host bits set in a network address).
	NetInvalid = errors.New("invalid Network")

	// FamilyInvalid is returned when an IP-family selector does not
	// resolve to a supported family (ipv4 or ipv6).
	FamilyInvalid = errors.New("invalid IP Family")
)
