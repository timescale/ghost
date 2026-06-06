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

// Rows is the result of running a query. It contains a row iterator in the
// style of [sql.Rows], as well as additional methods for getting column data,
// metadata about the query execution, and the number of rows affected. Not all
// functionality is supported by all [Driver] implementations.
type Rows interface {
	// See [sql.Rows.Next] documentation.
	Next() bool

	// See [sql.Rows.Scan] documentation.
	Scan(dest ...any) error

	// See [sql.Rows.Err] documentation.
	Err() error

	// See [sql.Rows.Close] documentation.
	Close() error

	// Columns returns information about the query result columns being
	// returned, including their names, types, and other column metadata. Also
	// identifies the Go type into which columns values should be scanned, and
	// provides convenience methods useful during scanning.
	Columns() (Columns, error)

	// RowsAffected returns the number of rows affected by the query (e.g. for
	// INSERT/UPDATE/DELETE statements), if supported by the underlying
	// [Driver] implementation. Returns nil if not supported or if the number
	// of rows affected is not available. For SELECT statements and other
	// statements that return rows, it may return the resulting row count. Note
	// that this method may return nil if called before the query has
	// completed. It is typically best to wait until the rows iterator has been
	// closed before calling it.
	RowsAffected(ctx context.Context) (*int64, error)
}

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

type scanTypeFn func(columnType *sql.ColumnType) reflect.Type

type baseRows struct {
	*sql.Rows
	scanTypeFn scanTypeFn
}

func (r *baseRows) Columns() (Columns, error) {
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
			columns[i] = r.buildColumn(deduper, ct)
		}
	}
	for i, ct := range columnTypes {
		if ct.Name() == "" {
			columns[i] = r.buildColumn(deduper, ct)
		}
	}
	return columns, nil
}

func (r *baseRows) buildColumn(deduper deduper, ct *sql.ColumnType) api.Column {
	scanType := r.scanTypeFn(ct)
	column := api.Column{
		Name:     deduper.dedupe(ct),
		Type:     ct.DatabaseTypeName(),
		Object:   scanType == types.JSONPtrType,
		Numeric:  scanType == types.NumericPtrType,
		ScanType: scanType,
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

func (r *baseRows) RowsAffected(ctx context.Context) (*int64, error) {
	return nil, nil
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

	// Some database types (in particular, Microsoft SQL Server) are capable of
	// returning empty column names. This throws off the DuckDB results cache,
	// which cannot handle columns without names. We therefore convert them to
	// a placeholder value instead.
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
