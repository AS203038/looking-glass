package errs

import (
	"errors"
)

var (
	AuthFailed       = errors.New("authentication error")
	ExecFailed       = errors.New("execution error")
	ConnectionFailed = errors.New("connection error")
)
