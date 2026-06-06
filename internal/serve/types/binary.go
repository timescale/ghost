package types

import (
	"encoding/hex"
	"fmt"
)

// Binary is a type that represents binary data in standard Postgres hex format.
// See https://www.postgresql.org/docs/current/datatype-binary.html#DATATYPE-BINARY-BYTEA-HEX-FORMAT
type Binary string

// Scan implements the [sql.Scanner] interface. It converts a []byte
// value to a string containing a hex representation of the value.
func (b *Binary) Scan(src any) error {
	switch val := src.(type) {
	case []byte:
		hex := hex.EncodeToString(val)
		*b = Binary(fmt.Sprintf(`\x%s`, hex))
	default:
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type Binary", src)
	}
	return nil
}
