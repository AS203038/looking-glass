package errs

import (
	"errors"
)

var (
	IPInvalid     = errors.New("invalid IP")
	NetInvalid    = errors.New("invalid Network")
	FamilyInvalid = errors.New("invalid IP Family")
)
