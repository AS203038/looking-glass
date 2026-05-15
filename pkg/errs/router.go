package errs

import (
	"errors"
)

// UnknownRouter is returned when the referenced router type is not registered.
var UnknownRouter = errors.New("router unknown")

// RouterUnavailable is returned when the requested router is unreachable or not found.
var RouterUnavailable = errors.New("router unavailable")

// OperationUnknown is returned when the router template defines no command for the requested operation.
var OperationUnknown = errors.New("operation unknown")
