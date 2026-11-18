package errs

import (
	"errors"
)

// ASPathMalformed is returned when an AS-path regex contains disallowed characters or fails to compile.
var ASPathMalformed = errors.New("AS Path malformed")

// ASPathEmpty is returned when an AS-path regex is empty after trimming.
var ASPathEmpty = errors.New("AS Path empty")

// ASPathTooLong is returned when an AS-path regex exceeds the sanitiser length limit.
var ASPathTooLong = errors.New("AS Path too long")

// CommunityMalformed is returned when a BGP standard community (RFC 1997)
// has an ASN or value outside the 16-bit unsigned range.
var CommunityMalformed = errors.New("BGP community malformed")

// PeerNameMalformed is returned when a BGP peer/protocol name contains disallowed characters.
var PeerNameMalformed = errors.New("BGP peer name malformed")

// PeerIPRequired is returned when the router command for a request needs the BGP peer address and the request has none.
var PeerIPRequired = errors.New("BGP peer IP required by this router")

// PeerNameRequired is returned when the router command for a request needs the BGP peer name and the request has none.
var PeerNameRequired = errors.New("BGP peer name required by this router")
