// Package dbdriver wraps database/sql + pgx to give us OID-aware column
// scan-type inference, server-side query cancellation, and a Postgres error
// normalizer suitable for projecting into the wire format the query widget
// expects.
//
// It is Postgres-only: no SSH tunneling, no multi-driver adapter registry, and
// no logging side-effects (callers log Close errors etc.).
package dbdriver

import (
	"errors"
	"reflect"
)

// Column carries column metadata to the widget. JSON shape matches the
// "Column" type the query widget's client consumes.
type Column struct {
	Name      string       `json:"name"`
	Type      string       `json:"type,omitempty"`
	Length    int64        `json:"length,omitempty"`
	Precision int64        `json:"precision,omitempty"`
	Scale     int64        `json:"scale,omitempty"`
	Object    bool         `json:"isObject,omitempty"`
	Numeric   bool         `json:"isNumeric,omitempty"`
	ScanType  reflect.Type `json:"-"`
}

// NormalizedError is the canonical error shape consumed by the widget. The
// JSON shape mirrors the widget client's ApiFailedResult error.
type NormalizedError struct {
	Code     string `json:"code,omitempty"`
	Column   int32  `json:"column,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Line     int32  `json:"line,omitempty"`
	Message  string `json:"message"`
	Position int32  `json:"position,omitempty"`

	Source string `json:"source"`

	Connect bool `json:"connect,omitempty"`
	Fatal   bool `json:"fatal,omitempty"`
	Timeout bool `json:"timeout,omitempty"`
	Cancel  bool `json:"cancel,omitempty"`
}

func (e *NormalizedError) Error() string { return e.Message }

// ErrMultiStatement is returned when multiple statements are sent in a single
// prepared (extended-protocol) call, which Postgres rejects. Multi-statement
// editor text is handled by running the widget-supplied statements one at a
// time (see streamQuery), so this only fires if a single statement itself
// contains multiple commands.
var ErrMultiStatement = errors.New("cannot run multiple statements in a single query")
