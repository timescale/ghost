package driver

import (
	"errors"
	"fmt"
)

var ErrMultiStatement = errors.New("cannot run multiple statements in a single query")

// InvalidClientTypeError is the error type returned when an invalid/unknown
// client type is given.
type InvalidClientTypeError struct {
	Client string
}

// Error implements the error interface.
func (e *InvalidClientTypeError) Error() string {
	return fmt.Sprintf("invalid client type: %s", e.Client)
}
