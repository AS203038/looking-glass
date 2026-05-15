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
