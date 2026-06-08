package driver

import (
	"errors"
)

var ErrMultiStatement = errors.New("cannot run multiple statements in a single query")
