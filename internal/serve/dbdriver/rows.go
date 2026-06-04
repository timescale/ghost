package dbdriver

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/timescale/ghost/internal/serve/dbtypes"
)

// Rows wraps [sql.Rows] with column metadata + accessors for row-affected
// counts. Always close after iteration.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error

	Columns() (Columns, error)
	RowsAffected(ctx context.Context) (*int64, error)
}

// Columns is a convenience wrapper around []Column.
type Columns []Column

// ScanTypes returns the Go types each column's value will be scanned into.
func (c Columns) ScanTypes() []reflect.Type {
	out := make([]reflect.Type, len(c))
	for i, column := range c {
		out[i] = column.ScanType
	}
	return out
}

// ScanTargets is a slice of newly-allocated pointers suitable for passing to
// Rows.Scan().
type ScanTargets []any

// ScanTargets allocates fresh scan targets for each column.
func (c Columns) ScanTargets() ScanTargets {
	targets := make(ScanTargets, len(c))
	for i, column := range c {
		targets[i] = reflect.New(column.ScanType).Interface()
	}
	return targets
}

// Values dereferences the scan targets after a call to Rows.Scan.
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

	// Two passes: named columns first so unnamed columns don't claim names
	// that the database actually produced.
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

func (r *baseRows) buildColumn(deduper deduper, ct *sql.ColumnType) Column {
	scanType := r.scanTypeFn(ct)
	column := Column{
		Name:     deduper.dedupe(ct),
		Type:     ct.DatabaseTypeName(),
		Object:   scanType == dbtypes.JSONPtrType,
		Numeric:  scanType == dbtypes.NumericPtrType,
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

func (r *baseRows) RowsAffected(ctx context.Context) (*int64, error) { return nil, nil }

type deduper map[string]int

func newDeduper(columnTypes []*sql.ColumnType) deduper {
	d := deduper{}
	for _, ct := range columnTypes {
		d[d.columnKey(ct.Name())] = 0
	}
	return d
}

func (d deduper) columnKey(name string) string { return strings.ToLower(name) }

func (d deduper) columnName(ct *sql.ColumnType) string {
	name := ct.Name()
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
