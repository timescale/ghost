package api

import (
	"encoding/json"
	"time"
)

// Duration is a wrapper around time.Duration that allows it to be
// marshalled to/from JSON via the standard duration string format.
type Duration struct {
	value time.Duration
}

// NewDuration returns a new [Duration] initialized with the provided
// [time.Duration] value.
func NewDuration(value time.Duration) Duration {
	return Duration{value: value}
}

// NewDuration returns a pointer to a new [Duration] initialized with the
// provided [time.Duration] value. Returns nil if the provided [time.Duration]
// is nil.
func NewDurationPtr(value *time.Duration) *Duration {
	if value == nil {
		return nil
	}

	d := NewDuration(*value)
	return &d
}

// Value returns the [Duration]'s value as a pointer to a [time.Duration]. If
// the Duration itself is nil, it returns nil.
func (d *Duration) Value() *time.Duration {
	if d == nil {
		return nil
	}
	return &d.value
}

// Implements the [json.Marshaler] interface.
func (d Duration) MarshalJSON() ([]byte, error) {
	str := d.value.String()
	return json.Marshal(str)
}
