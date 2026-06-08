package driver

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/types"
)

// Columns is a slice of Column types, with some additional convenience
// methods.
type Columns []api.Column

// ScanTypes returns a [reflect.Type] slice representing the Go types into
// which column values should be scanned.
func (c Columns) ScanTypes() []reflect.Type {
	types := make([]reflect.Type, len(c))
	for i, column := range c {
		types[i] = column.ScanType
	}
	return types
}

// ScanTargets represents a slice of types that can be passed to [Rows.Scan] to
// scan column values from the database, for a given set of query results.
type ScanTargets []any

// ScanTargets returns a slice of types suitable for passing to [Rows.Scan].
func (c Columns) ScanTargets() ScanTargets {
	targets := make(ScanTargets, len(c))
	for i, column := range c {
		targets[i] = reflect.New(column.ScanType).Interface()
	}
	return targets
}

// Values returns the row values that have been scanned into [ScanTargets]
// after a call to [Rows.Scan].
func (s ScanTargets) Values() []any {
	vals := make([]any, len(s))
	for i, target := range s {
		vals[i] = reflect.ValueOf(target).Elem().Interface()
	}
	return vals
}

// Rows is the result of running a query. It embeds [sql.Rows] for
// iteration/scanning (Next, Scan, Err, Close) and adds methods for column
// metadata and the number of rows affected, the latter sourced from the query
// tracer.
type Rows struct {
	*sql.Rows
	tracer *postgresQueryTracer
}

// Columns returns information about the query result columns being returned,
// including their names, types, and other column metadata. It also identifies
// the Go type into which column values should be scanned.
func (r *Rows) Columns() (Columns, error) {
	columnTypes, err := r.ColumnTypes()
	if err != nil {
		return nil, err
	}

	columns := make(Columns, len(columnTypes))
	deduper := newDeduper(columnTypes)

	// First build all columns with non-empty names, then build all columns
	// with empty names. Doing it in two passes ensures we do our best to
	// maintain the column names that were returned from the database (i.e.
	// that we don't use 'column' for an empty column when 'column' is already
	// used by another field returned from the database).
	for i, ct := range columnTypes {
		if ct.Name() != "" {
			columns[i] = buildColumn(deduper, ct)
		}
	}
	for i, ct := range columnTypes {
		if ct.Name() == "" {
			columns[i] = buildColumn(deduper, ct)
		}
	}
	return columns, nil
}

// RowsAffected returns the number of rows affected by the query (e.g. for
// INSERT/UPDATE/DELETE statements), if available. Returns nil if the number of
// rows affected is not available. For SELECT statements and other statements
// that return rows, it may return the resulting row count. Note that this
// method may return nil if called before the query has completed. It is
// typically best to wait until the rows iterator has been closed before calling
// it.
func (r *Rows) RowsAffected(ctx context.Context) (*int64, error) {
	if r.tracer.lastCommandTag != nil {
		rowsAffected := r.tracer.lastCommandTag.RowsAffected()
		return &rowsAffected, nil
	}
	return nil, nil
}

func buildColumn(deduper deduper, ct *sql.ColumnType) api.Column {
	st := scanType(ct)
	column := api.Column{
		Name:     deduper.dedupe(ct),
		Type:     ct.DatabaseTypeName(),
		Object:   st == types.JSONPtrType,
		Numeric:  st == types.NumericPtrType,
		ScanType: st,
	}
	if length, ok := ct.Length(); ok {
		column.Length = length
	}
	if precision, scale, ok := ct.DecimalSize(); ok {
		column.Precision = precision
		column.Scale = scale
	}
	return column
}

// scanType identifies the Go type into which a column's values should be
// scanned. Postgres-specific database types are mapped first; everything else
// falls back to normalizing the driver's reported scan type.
func scanType(columnType *sql.ColumnType) reflect.Type {
	switch columnType.DatabaseTypeName() {
	case "JSON", "JSONB":
		return types.JSONPtrType
	case "NUMERIC":
		// Maintain exact precision by scanning as a types.Number (which also
		// supports special values like NaN and Infinity/-Infinity).
		return types.NumericPtrType
	case "BYTEA":
		// Represent binary types in standard Postgres hex format.
		return types.BinaryPtrType
	case "DATE":
		// The stdlib adapter scans dates into time.Time values, which add time
		// and time zone information when output as a string. Scan into custom
		// Date type instead to keep the plain date format.
		return types.DatePtrType
	case "TIMESTAMP":
		// The stdlib adapter scans dates into time.Time values, which add time
		// zone information when output as a string. Scan into custom DateTime
		// type instead to keep the plain timestamp format.
		return types.DateTimePtrType
	case "TIMESTAMPTZ":
		// Date types can be Infinity/-Infinity, which cannot be represented in
		// a time.Time value, so scan them as strings.
		return types.TimestampPtrType
	}

	t := columnType.ScanType()
	switch t {
	// NOTE: Some drivers return sql.NullWhatever (or *sql.NullWhatever) types,
	// which don't serialize to JSON well. A pointer to a built-in type works
	// just as well for scanning nullable values.
	case types.NullBoolType, types.NullBoolPtrType:
		t = types.BoolType
	case types.NullByteType, types.NullBytePtrType:
		t = types.ByteType
	case types.NullFloat64Type, types.NullFloat64PtrType:
		t = types.Float64Type
	case types.NullInt16Type, types.NullInt16PtrType:
		t = types.Int16Type
	case types.NullInt32Type, types.NullInt32PtrType:
		t = types.Int32Type
	case types.NullInt64Type, types.NullInt64PtrType:
		t = types.Int64Type
	case types.NullStringType, types.NullStringPtrType:
		t = types.StringType
	case types.NullTimeType, types.NullTimePtrType:
		t = types.TimeType
	case types.RawBytesType:
		// The sql.RawBytes type is not safe if you aren't sure how long the
		// memory will be needed for. A standard []byte is safer and more
		// consistent.
		t = types.BytesType
	case nil:
		// Some non-compliant drivers will sometimes return nil for the scan
		// type, instead of any. Fix that here.
		t = types.AnyType
	}

	switch t.Kind() {
	case reflect.Pointer, reflect.Interface:
	default:
		// Return pointer for sake of scanning NULLs.
		t = reflect.PointerTo(t)
	}
	return t
}

type deduper map[string]int

func newDeduper(columnTypes []*sql.ColumnType) deduper {
	// Initialize map with all known columns set to 0, so we can be sure not to
	// use a pre-existing column name when de-duping (e.g. if a column was
	// originally named field_2, for example).
	d := deduper{}
	for _, ct := range columnTypes {
		d[d.columnKey(ct.Name())] = 0
	}
	return d
}

// Some database types support case-sensitive column names (usually by quoting
// them in double-quotes). However, DuckDB does not. Returning column names that
// only differ in their case therefore breaks the DuckDB results cache. For
// that reason, we dedupe column names in a case-insensitive way.
func (d deduper) columnKey(name string) string {
	return strings.ToLower(name)
}

func (d deduper) columnName(ct *sql.ColumnType) string {
	name := ct.Name()

	// Some database types are capable of returning empty column names. This
	// throws off the DuckDB results cache, which cannot handle columns without
	// names. We therefore convert them to a placeholder value instead.
	if name == "" {
		name = "column"
	}

	return name
}

func (d deduper) dedupe(ct *sql.ColumnType) string {
	name := d.columnName(ct)
	key := d.columnKey(name)

	count := d[key]
	if count == 0 {
		d[key] = 1
		return name
	}

	for {
		newName := fmt.Sprintf("%s_%d", name, count)
		newKey := d.columnKey(newName)
		count++
		if _, exists := d[newKey]; !exists {
			d[key] = count
			return newName
		}
	}
}
