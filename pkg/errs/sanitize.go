package errs

import (
	"errors"
)

var (
	ASPathMalformed = errors.New("AS Path malformed")
	ASPathEmpty     = errors.New("AS Path empty")
	ASPathTooLong   = errors.New("AS Path too long")
)
