package function

import (
	"encoding/json"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// textResults asks for every result column in the wire protocol's text
// format (a single format code applies to all columns). PostgreSQL renders
// each value with its canonical output function, and pgx's codecs can still
// decode text into native Go types where we want them (bool, []byte,
// pgtype.Timestamp); everything else scans verbatim into strings. Text
// format is also what makes unknown types work: any type the server can
// output - including user-defined enums, composites, and domains - scans
// into a string, and pgx parses array literals of unknown element types when
// the destination is a slice.
var textResults = pgx.QueryResultFormats{pgtype.TextFormatCode}

// pgNumber holds the text rendering of a PostgreSQL number (integer, float,
// or numeric) and marshals it as a JSON number verbatim, preserving exact
// precision with no float64 round-trip. NaN and ±Infinity - which JSON
// numbers cannot represent - marshal as strings.
type pgNumber string

func (n pgNumber) MarshalJSON() ([]byte, error) {
	switch n {
	case "NaN", "Infinity", "-Infinity":
		return json.Marshal(string(n))
	}

	// Marshaling through json.Number validates that the text is a valid JSON
	// number literal, so a misclassified column fails loudly instead of
	// silently emitting JSON that violates the tool's schema.
	return json.Marshal(json.Number(n))
}

// Scan destination types. Each is chosen so the scanned value marshals
// directly to the JSON its tool schema describes, with no conversion after
// scanning. Pointer and nilable types scan NULL as nil, which marshals as
// JSON null; pgtype.Timestamp(tz) marshals NULL, RFC 3339, and PostgreSQL's
// infinite timestamps itself.
var (
	stringPtrType   = reflect.TypeFor[*string]()
	numberPtrType   = reflect.TypeFor[*pgNumber]()
	boolPtrType     = reflect.TypeFor[*bool]()
	rawJSONType     = reflect.TypeFor[json.RawMessage]()
	byteSliceType   = reflect.TypeFor[[]byte]()
	timestampType   = reflect.TypeFor[pgtype.Timestamp]()
	timestamptzType = reflect.TypeFor[pgtype.Timestamptz]()
)

// scanTypes returns the Go type each result column is scanned into, chosen
// from the introspected column metadata - the same metadata the tool schemas
// are built from, so values and schemas agree by construction.
func scanTypes(cols []column) []reflect.Type {
	types := make([]reflect.Type, len(cols))
	for i, col := range cols {
		t := baseScanType(col.Type)
		if col.Type.IsArray {
			// Array elements scan into the scalar type; NULL elements become
			// nil/invalid elements, which marshal as JSON null.
			// Multidimensional arrays scan flattened.
			t = reflect.SliceOf(t)
		}
		types[i] = t
	}
	return types
}

// baseScanType picks the scan type for a single (non-array) value of the
// column's PostgreSQL type. Types without a specific entry - including
// user-defined and unknown types - scan into strings, matching their
// string-typed schema.
func baseScanType(typ typeInfo) reflect.Type {
	switch typ.Name {
	case "smallint", "int2", "smallserial", "serial2",
		"integer", "int", "int4", "serial", "serial4",
		"bigint", "int8", "bigserial", "serial8", "oid",
		"real", "float4", "double precision", "float8", "float",
		"numeric", "decimal":
		return numberPtrType
	case "boolean", "bool":
		return boolPtrType
	case "json", "jsonb":
		return rawJSONType
	case "bytea":
		return byteSliceType
	case "timestamp", "timestamp without time zone":
		return timestampType
	case "timestamptz", "timestamp with time zone":
		return timestamptzType
	default:
		return stringPtrType
	}
}

// scanDests returns fresh scan destinations (pointers to zero values of the
// scan types) for one row.
func scanDests(types []reflect.Type) []any {
	dests := make([]any, len(types))
	for i, t := range types {
		dests[i] = reflect.New(t).Interface()
	}
	return dests
}

// rowToMap converts one scanned row into the result object keyed by column
// name. The scanned values marshal to schema-conforming JSON on their own,
// so this only dereferences the destinations.
func rowToMap(fieldDescs []pgconn.FieldDescription, dests []any) map[string]any {
	result := make(map[string]any, len(dests))
	for i, dest := range dests {
		result[fieldDescs[i].Name] = reflect.ValueOf(dest).Elem().Interface()
	}
	return result
}
