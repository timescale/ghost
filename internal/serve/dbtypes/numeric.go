package dbtypes

import "encoding/json"

// Numeric represents arbitrary-precision decimal values as well as special
// values like Infinity, -Infinity, and NaN.
type Numeric string

// MarshalJSON marshals the underlying string as a json.Number when possible
// (i.e. as a number without quotes), falling back to a string when the value
// is not a valid JSON number (e.g. Postgres's Infinity, -Infinity, NaN).
func (n Numeric) MarshalJSON() ([]byte, error) {
	out, err := json.Marshal(json.Number(n))
	if err != nil {
		return json.Marshal(string(n))
	}
	return out, err
}
