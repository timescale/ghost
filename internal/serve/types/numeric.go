package types

import "encoding/json"

// Numeric is a type capable of representing arbitrary precision decimal types,
// as well as special values like Infinity, -Infinity, and NaN.
type Numeric string

// MarshalJSON implements the [json.Marshaler] interface. It attempts to
// marshal the underlying string to JSON via [json.Number] (i.e. as a number,
// without quotes), but falls back to marshalling it as a string (which differs
// from the [json.Number] behavior). This accounts for edge cases like
// Postgres's Infinity, -Infinity, and NaN values.
func (n Numeric) MarshalJSON() ([]byte, error) {
	out, err := json.Marshal(json.Number(n))
	if err != nil {
		return json.Marshal(string(n))
	}
	return out, err
}
