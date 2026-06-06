package types

import (
	"encoding/json"
	"fmt"
)

// JSON is a type capable of representing arbitrary JSON values without having
// to marshal/unmarshal them into corresponding Go types.
type JSON string

// MarshalJSON implements the [json.Marshaler] interface. It marshals the
// underlying []byte as a [json.RawMessage] (i.e. as a literal JSON value,
// without quotes).
func (j JSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(json.RawMessage(j))
}

// Scan implements the [sql.Scanner] interface. It interprets either a []byte
// or string value as raw JSON.
func (j *JSON) Scan(src any) error {
	switch val := src.(type) {
	case string:
		*j = JSON(val)
	case []byte:
		// NOTE: Casting to a string copies the byte slice passed in. This is
		// critical, as some database drivers (e.g. the MySQL driver) will pass
		// byte slices owned by the underlying driver, which are only valid
		// until the next call to Rows.Next(), Rows.Scan(), or Rows.Close().
		// See: https://pkg.go.dev/database/sql#Scanner
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
