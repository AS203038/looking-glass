package errs

import (
	"errors"
)

// Input-sanitisation sentinels. Returned by the validators in
// pkg/utils/sanitize.go before user-supplied strings are interpolated
// into router commands; their primary purpose is to prevent SSH
// command injection and ReDoS-style abuse of BGP filters.
var (
	// ASPathMalformed is returned when an AS-path regex contains
	// characters outside the sanitiser's allow-list, or fails to
	// compile as a Go regular expression.
	ASPathMalformed = errors.New("AS Path malformed")

	// ASPathEmpty is returned when an AS-path regex is empty or
	// reduces to whitespace after trimming.
	ASPathEmpty = errors.New("AS Path empty")

	// ASPathTooLong is returned when an AS-path regex exceeds the
	// hard length limit enforced by the sanitiser.
	ASPathTooLong = errors.New("AS Path too long")
)
