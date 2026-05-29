// Package dbdriver wraps database/sql + pgx to give us OID-aware column
// scan-type inference, server-side query cancellation, and a Postgres error
// normalizer suitable for projecting into the wire format the
// popsql-query-widget expects.
//
// This is a trimmed-down port of github.com/timescale/popsql-query's
// internal/driver package — Postgres only, no SSH tunneling, no multi-driver
// adapter registry, and no logging side-effects.
package dbdriver

import (
	"errors"
	"reflect"
)

// ColumnCase controls how column names are presented to the widget. Today we
// always emit them as-is.
type ColumnCase string

const (
	ColumnCaseDefault ColumnCase = ""
	ColumnCaseLower   ColumnCase = "lower"
	ColumnCaseUpper   ColumnCase = "upper"
)

// Column carries column metadata to the widget. JSON shape matches the
// "Column" type defined by @popsql/types and consumed by
// popsql-query-widget's TimescaleQueryClient.
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

// Metadata is reserved for future use; popsql-query's Metadata is only
// populated by BigQuery's bytes-processed counter.
type Metadata struct {
	BytesProcessed int64 `json:"bytesProcessed"`
}

// NormalizedError is the canonical error shape consumed by the widget. The
// JSON shape mirrors @popsql/types' ApiFailedResult error.
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

// ErrMultiStatement is returned when the user attempts to run multiple
// statements in a single query. Multi-statement support requires either an
// extended-protocol round trip per statement or pgx's "simple protocol" mode,
// which interpolates parameters client-side. For now we follow popsql-query's
// posture and reject the case.
var ErrMultiStatement = errors.New("cannot run multiple statements in a single query")
