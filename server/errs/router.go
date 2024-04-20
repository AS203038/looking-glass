package errs

import (
	"errors"
)

var (
	UnknownRouter     = errors.New("router unknown")
	RouterUnavailable = errors.New("router unavailable")
)
