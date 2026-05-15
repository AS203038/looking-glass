// Package errs defines the sentinel errors surfaced by the Looking Glass backend.
package errs

import (
	"errors"
)

// IPInvalid is returned when an input string cannot be parsed as an IPv4 or IPv6 address.
var IPInvalid = errors.New("invalid IP")

// NetInvalid is returned when an input string cannot be parsed as a CIDR network.
var NetInvalid = errors.New("invalid Network")

// FamilyInvalid is returned when an IP-family selector is not ipv4 or ipv6.
var FamilyInvalid = errors.New("invalid IP Family")
