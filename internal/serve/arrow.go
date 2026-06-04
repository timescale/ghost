package serve

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/timescale/ghost/internal/serve/dbdriver"
	"github.com/timescale/ghost/internal/serve/dbtypes"
)

// Arrow IPC encoding for query result sets. The schema metadata and the
// synthetic __popsql_row_num__ column are required by the widget's table
// renderer, which expects the same Arrow wire contract as the hosted query
// service.

const columnsMetadataKey = "__popsql_columns__"

var (
	rowNumField = arrow.Field{
		Name: "__popsql_row_num__",
		Type: arrow.PrimitiveTypes.Int64,
	}
	rowNumBuilderFn = basicBuilderFn[*array.Int64Builder, int64]
)

// arrowBuilder wraps an array.Builder and exposes AppendValue, which accepts
// values of type 'any' and routes them through a column-specific builderFn.
type arrowBuilder interface {
	array.Builder
	AppendValue(val any) error
}

type builderFn func(builder array.Builder, val any) error

type columnBuilder struct {
	array.Builder
	fn builderFn
}

func (c *columnBuilder) AppendValue(val any) error { return c.fn(c.Builder, val) }

// RecordBuilder is a thin wrapper around array.RecordBuilder that appends a
// synthetic row-number column and exposes AppendRow for []any values.
type RecordBuilder struct {
	*array.RecordBuilder
	fields         []arrowBuilder
	recordRowCount int64
	totalRowCount  int64
}

// NewRecordBuilder builds an Arrow schema from the supplied columns and
// returns a RecordBuilder ready to append rows.
func NewRecordBuilder(columns dbdriver.Columns) (*RecordBuilder, error) {
	schema, builderFns, err := arrowSchema(columns)
	if err != nil {
		return nil, err
	}

	rb := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	fields := make([]arrowBuilder, schema.NumFields())
	for i, field := range rb.Fields() {
		fields[i] = &columnBuilder{Builder: field, fn: builderFns[i]}
	}
	return &RecordBuilder{RecordBuilder: rb, fields: fields}, nil
}

// AppendRow appends a single row + populates the synthetic row-num column.
// The row must contain one entry per column in the same order as the
// dbdriver.Columns passed to NewRecordBuilder.
func (rb *RecordBuilder) AppendRow(row []any) error {
	for i, val := range row {
		if err := rb.fields[i].AppendValue(val); err != nil {
			return err
		}
	}
	if err := rb.fields[len(row)].AppendValue(rb.totalRowCount); err != nil {
		return err
	}
	rb.recordRowCount++
	rb.totalRowCount++
	return nil
}

// RecordRowCount returns the number of rows accumulated in the in-progress
// record batch (reset to 0 by NewRecordBatch).
func (rb *RecordBuilder) RecordRowCount() int64 { return rb.recordRowCount }

// NewRecordBatch finalizes the in-progress record and resets the row counter.
func (rb *RecordBuilder) NewRecordBatch() arrow.RecordBatch {
	rb.recordRowCount = 0
	return rb.RecordBuilder.NewRecordBatch()
}

func arrowSchema(columns dbdriver.Columns) (*arrow.Schema, []builderFn, error) {
	fields := make([]arrow.Field, len(columns)+1)
	builderFns := make([]builderFn, len(columns)+1)
	for i, column := range columns {
		arrowType, builderFn := arrowType(column)
		fields[i] = arrow.Field{
			Name:     column.Name,
			Type:     arrowType,
			Nullable: true,
		}
		builderFns[i] = builderFn
	}
	fields[len(columns)] = rowNumField
	builderFns[len(columns)] = rowNumBuilderFn

	columnJSON, err := json.Marshal(columns)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling columns to JSON: %w", err)
	}
	metadata := arrow.NewMetadata(
		[]string{columnsMetadataKey},
		[]string{string(columnJSON)},
	)
	return arrow.NewSchema(fields, &metadata), builderFns, nil
}

var (
	boolBuilderFn        = basicBuilderFn[*array.BooleanBuilder, bool]
	float32BuilderFn     = basicBuilderFn[*array.Float32Builder, float32]
	float64BuilderFn     = basicBuilderFn[*array.Float64Builder, float64]
	intBuilderFn         = convertBuilderFn[*array.Int64Builder](castToInt64[int])
	int8BuilderFn        = basicBuilderFn[*array.Int8Builder, int8]
	int16BuilderFn       = basicBuilderFn[*array.Int16Builder, int16]
	int32BuilderFn       = basicBuilderFn[*array.Int32Builder, int32]
	int64BuilderFn       = basicBuilderFn[*array.Int64Builder, int64]
	uintBuilderFn        = convertBuilderFn[*array.Uint64Builder](castToUint64[uint])
	uint8BuilderFn       = basicBuilderFn[*array.Uint8Builder, uint8]
	uint16BuilderFn      = basicBuilderFn[*array.Uint16Builder, uint16]
	uint32BuilderFn      = basicBuilderFn[*array.Uint32Builder, uint32]
	uint64BuilderFn      = basicBuilderFn[*array.Uint64Builder, uint64]
	stringBuilderFn      = basicBuilderFn[*array.StringBuilder, string]
	binaryBuilderFn      = basicBuilderFn[*array.BinaryBuilder, []byte]
	timeBuilderFn        = convertBuilderFn[*array.StringBuilder](timeToStr)
	dateBuilderFn        = convertBuilderFn[*array.StringBuilder](castToStr[dbtypes.Date])
	clockTimeBuilderFn   = convertBuilderFn[*array.StringBuilder](castToStr[dbtypes.ClockTime])
	clockTimeTZBuilderFn = convertBuilderFn[*array.StringBuilder](castToStr[dbtypes.ClockTimeTZ])
	dateTimeBuilderFn    = convertBuilderFn[*array.StringBuilder](castToStr[dbtypes.DateTime])
	timestampBuilderFn   = convertBuilderFn[*array.StringBuilder](castToStr[dbtypes.Timestamp])
	numericBuilderFn     = convertBuilderFn[*array.StringBuilder](castToStr[dbtypes.Numeric])
	jsonBuilderFn        = convertBuilderFn[*array.StringBuilder](castToStr[dbtypes.JSON])
	binaryStrBuilderFn   = convertBuilderFn[*array.StringBuilder](castToStr[dbtypes.Binary])
)

func arrowType(column dbdriver.Column) (arrow.DataType, builderFn) {
	switch column.ScanType {
	case dbtypes.BoolType, dbtypes.BoolPtrType:
		return arrow.FixedWidthTypes.Boolean, boolBuilderFn
	case dbtypes.Float32Type, dbtypes.Float32PtrType:
		return arrow.PrimitiveTypes.Float32, float32BuilderFn
	case dbtypes.Float64Type, dbtypes.Float64PtrType:
		return arrow.PrimitiveTypes.Float64, float64BuilderFn
	case dbtypes.IntType, dbtypes.IntPtrType:
		return arrow.PrimitiveTypes.Int64, intBuilderFn
	case dbtypes.Int8Type, dbtypes.Int8PtrType:
		return arrow.PrimitiveTypes.Int8, int8BuilderFn
	case dbtypes.Int16Type, dbtypes.Int16PtrType:
		return arrow.PrimitiveTypes.Int16, int16BuilderFn
	case dbtypes.Int32Type, dbtypes.Int32PtrType:
		return arrow.PrimitiveTypes.Int32, int32BuilderFn
	case dbtypes.Int64Type, dbtypes.Int64PtrType:
		return arrow.PrimitiveTypes.Int64, int64BuilderFn
	case dbtypes.UintType, dbtypes.UintPtrType:
		return arrow.PrimitiveTypes.Uint64, uintBuilderFn
	case dbtypes.Uint8Type, dbtypes.Uint8PtrType:
		return arrow.PrimitiveTypes.Uint8, uint8BuilderFn
	case dbtypes.Uint16Type, dbtypes.Uint16PtrType:
		return arrow.PrimitiveTypes.Uint16, uint16BuilderFn
	case dbtypes.Uint32Type, dbtypes.Uint32PtrType:
		return arrow.PrimitiveTypes.Uint32, uint32BuilderFn
	case dbtypes.Uint64Type, dbtypes.Uint64PtrType:
		return arrow.PrimitiveTypes.Uint64, uint64BuilderFn
	case dbtypes.StringType, dbtypes.StringPtrType:
		return arrow.BinaryTypes.String, stringBuilderFn
	case dbtypes.BytesType, dbtypes.BytesPtrType:
		return arrow.BinaryTypes.Binary, binaryBuilderFn
	case dbtypes.TimeType, dbtypes.TimePtrType:
		return arrow.BinaryTypes.String, timeBuilderFn
	case dbtypes.DateType, dbtypes.DatePtrType:
		return arrow.BinaryTypes.String, dateBuilderFn
	case dbtypes.ClockTimeType, dbtypes.ClockTimePtrType:
		return arrow.BinaryTypes.String, clockTimeBuilderFn
	case dbtypes.ClockTimeTZType, dbtypes.ClockTimeTZPtrType:
		return arrow.BinaryTypes.String, clockTimeTZBuilderFn
	case dbtypes.DateTimeType, dbtypes.DateTimePtrType:
		return arrow.BinaryTypes.String, dateTimeBuilderFn
	case dbtypes.TimestampType, dbtypes.TimestampPtrType:
		return arrow.BinaryTypes.String, timestampBuilderFn
	case dbtypes.NumericType, dbtypes.NumericPtrType:
		return arrow.BinaryTypes.String, numericBuilderFn
	case dbtypes.JSONType, dbtypes.JSONPtrType:
		return arrow.BinaryTypes.String, jsonBuilderFn
	case dbtypes.BinaryType, dbtypes.BinaryPtrType:
		return arrow.BinaryTypes.String, binaryStrBuilderFn
	}
	return arrow.BinaryTypes.String, unknownBuilderFn
}

type arrowAppender[T any] interface {
	Append(value T)
	AppendNull()
}

func basicBuilderFn[A arrowAppender[T], T any](builder array.Builder, value any) error {
	b := builder.(A)
	switch val := (value).(type) {
	case nil:
		b.AppendNull()
	case T:
		b.Append(val)
	case *T:
		if val == nil {
			b.AppendNull()
		} else {
			b.Append(*val)
		}
	default:
		return fmt.Errorf("arrow: cannot append %T as %T", value, *new(T))
	}
	return nil
}

func convertBuilderFn[A arrowAppender[T], V any, T any](convert func(V) T) builderFn {
	return func(builder array.Builder, value any) error {
		b := builder.(A)
		switch val := (value).(type) {
		case nil:
			builder.AppendNull()
		case V:
			b.Append(convert(val))
		case *V:
			if val == nil {
				builder.AppendNull()
			} else {
				b.Append(convert(*val))
			}
		default:
			return fmt.Errorf("arrow: cannot append %T as %T", value, *new(T))
		}
		return nil
	}
}

func timeToStr(value time.Time) string { return value.Format(time.RFC3339Nano) }

type stringish interface{ ~string | ~[]byte }

func castToStr[T stringish](value T) string { return string(value) }

type int64ish interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

func castToInt64[T int64ish](value T) int64 { return int64(value) }

type uint64ish interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

func castToUint64[T uint64ish](value T) uint64 { return uint64(value) }

func unknownBuilderFn(builder array.Builder, value any) error {
	b := builder.(*array.StringBuilder)
	switch val := value.(type) {
	case nil:
		b.AppendNull()
	case string:
		b.Append(val)
	case *string:
		if val == nil {
			b.AppendNull()
		} else {
			b.Append(*val)
		}
	case []byte:
		if val == nil {
			b.AppendNull()
		} else {
			b.Append(string(val))
		}
	case *[]byte:
		if val == nil || *val == nil {
			b.AppendNull()
		} else {
			b.Append(string(*val))
		}
	case *any:
		if val == nil {
			b.AppendNull()
		} else {
			return unknownBuilderFn(builder, *val)
		}
	default:
		if shouldMarshalJSON(reflect.TypeOf(val)) {
			if out, err := json.Marshal(val); err == nil {
				b.Append(string(out))
				return nil
			}
		}
		b.Append(fmt.Sprint(val))
	}
	return nil
}

// shouldMarshalJSON returns true for compound types (arrays, slices, maps,
// structs) that aren't sql.Scanner-compliant. Postgres rarely hits this path,
// but it's kept as a safe fallback for any driver value that can't be scanned
// directly into one of the primitive arrow builders.
func shouldMarshalJSON(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer:
		return shouldMarshalJSON(t.Elem())
	case reflect.Array, reflect.Slice, reflect.Map, reflect.Struct:
		return true
	default:
		return false
	}
}
