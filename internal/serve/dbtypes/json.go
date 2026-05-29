package dbtypes

import (
	"encoding/json"
	"fmt"
)

// JSON represents an arbitrary JSON value without unmarshalling it into a
// concrete Go type.
type JSON string

// MarshalJSON emits the underlying string as a literal JSON value.
func (j JSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(json.RawMessage(j))
}

// Scan accepts string, []byte, or already-decoded map/slice values and stores
// the raw JSON encoding.
func (j *JSON) Scan(src any) error {
	switch val := src.(type) {
	case string:
		*j = JSON(val)
	case []byte:
		// Casting to a string copies the byte slice, which is critical: some
		// drivers reuse the underlying buffer between Scan calls.
		*j = JSON(val)
	case map[string]any, []any:
		out, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("error marshalling %T to JSON: %w", src, err)
		}
		*j = JSON(out)
	default:
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type JSON", src)
	}
	return nil
}
