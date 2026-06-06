package writer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/driver"
	"github.com/timescale/ghost/internal/serve/types"
)

const columnsMetadataKey = "__popsql_columns__"

// These variables define a special internal row number field, which is
// included in all arrow schemas, and which allows us to maintain the original
// ordering of the rows as they came back from the database.
var (
	rowNumField = arrow.Field{
		Name: "__popsql_row_num__",
		Type: arrow.PrimitiveTypes.Int64,
	}
	rowNumBuilderFn = basicBuilderFn[*array.Int64Builder, int64]
)

// Builder is a type capable of building a single column/array of arrow values.
// It wraps arrow's [array.Builder] interface with an additional method that
// makes it possible to append values of type 'any'
type Builder interface {
	array.Builder
	AppendValue(val any) error
}

type builderFn func(builder array.Builder, val any) error

// builder implements the Builder interface. It does so via the help of a
// builderFn helper method, which takes an array.Builder and a value of type
// 'any' and attempts to append the value to the builder.
type builder struct {
	array.Builder
	fn builderFn
}

// AppendValue attempts to append the provided value to the underlying
// [array.Builder], converting it to the appropriate type if necessary.
func (b *builder) AppendValue(val any) error {
	return b.fn(b.Builder, val)
}

// RecordBuilder wraps arrow's [array.RecordBuilder] with additional methods.
// In particular, it adds the [RecordBuilder.AppendRow] method, which makes it
// possible to append rows of values of type 'any' to the [arrow.RecordBatch] being
// built.
type RecordBuilder struct {
	*array.RecordBuilder
	fields         []Builder
	recordRowCount int64
	totalRowCount  int64
}

// NewRecordBuilder generates an [arrow.Schema] for the provided [Columns] and
// returns a [RecordBuilder] capable of building records with that schema.
func NewRecordBuilder(columns driver.Columns) (*RecordBuilder, error) {
	schema, builderFns, err := arrowSchema(columns)
	if err != nil {
		return nil, err
	}

	rb := array.NewRecordBuilder(memory.DefaultAllocator, schema)

	fields := make([]Builder, schema.NumFields())
	for i, field := range rb.Fields() {
		fields[i] = &builder{
			Builder: field,
			fn:      builderFns[i],
		}
	}

	return &RecordBuilder{
		RecordBuilder: rb,
		fields:        fields,
	}, nil
}

// Field returns a [Builder] for the field/column in the specified position. It
// wraps the [array.RecordBuilder.Field] method.
func (rb *RecordBuilder) Field(i int) Builder {
	return rb.fields[i]
}

// AppendRow appends a row of values of type 'any' to the [array.Record]
// currently being built. The types of the row values must correspond to the
// column types passed to [NewRecordBuilder], as defined by [Column.ScanType].
func (rb *RecordBuilder) AppendRow(row []any) error {
	for i, val := range row {
		if err := rb.Field(i).AppendValue(val); err != nil {
			return err
		}
	}

	// Populate special "__popsql_row_num__" field.
	if err := rb.Field(len(row)).AppendValue(rb.totalRowCount); err != nil {
		return err
	}

	rb.recordRowCount++
	rb.totalRowCount++
	return nil
}

// RecordRowCount returns the number of rows in the [arrow.RecordBatch] currently
// being built.
func (rb *RecordBuilder) RecordRowCount() int64 {
	return rb.recordRowCount
}

// NewRecord returns the current [arrow.RecordBatch] and begins building a new one.
func (rb *RecordBuilder) NewRecord() arrow.RecordBatch {
	rb.recordRowCount = 0
	return rb.RecordBuilder.NewRecordBatch()
}

func arrowSchema(columns driver.Columns) (*arrow.Schema, []builderFn, error) {
	fields := make([]arrow.Field, len(columns)+1)
	builderFns := make([]builderFn, len(columns)+1)
	for i, column := range columns {
		arrowType, builderFn := arrowType(column)
		field := arrow.Field{
			Name:     column.Name,
			Type:     arrowType,
			Nullable: true,
		}
		fields[i] = field
		builderFns[i] = builderFn
	}

	// Append special "__popsql_row_num__" field to schema.
	fields[len(columns)] = rowNumField
	builderFns[len(columns)] = rowNumBuilderFn

	columnJSON, err := json.Marshal(columns)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshaling columns to JSON: %w", err)
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
	dateBuilderFn        = convertBuilderFn[*array.StringBuilder](castToStr[types.Date])
	clockTimeBuilderFn   = convertBuilderFn[*array.StringBuilder](castToStr[types.ClockTime])
	clockTimeTZBuilderFn = convertBuilderFn[*array.StringBuilder](castToStr[types.ClockTimeTZ])
	dateTimeBuilderFn    = convertBuilderFn[*array.StringBuilder](castToStr[types.DateTime])
	timestampBuilderFn   = convertBuilderFn[*array.StringBuilder](castToStr[types.Timestamp])
	numericBuilderFn     = convertBuilderFn[*array.StringBuilder](castToStr[types.Numeric])
	jsonBuilderFn        = convertBuilderFn[*array.StringBuilder](castToStr[types.JSON])
	binaryStrBuilderFn   = convertBuilderFn[*array.StringBuilder](castToStr[types.Binary])
)

// TODO: Add support for date, timestamp, time, and decimal types, all of which
// are challenging due precision/scale issues, timezone issues, and special
// values that cannot be represented in arrow (i.e. NaN, Infinity, -Infinity).
func arrowType(column api.Column) (arrow.DataType, builderFn) {
	switch column.ScanType {
	case types.BoolType, types.BoolPtrType:
		return arrow.FixedWidthTypes.Boolean, boolBuilderFn
	case types.Float32Type, types.Float32PtrType:
		return arrow.PrimitiveTypes.Float32, float32BuilderFn
	case types.Float64Type, types.Float64PtrType:
		return arrow.PrimitiveTypes.Float64, float64BuilderFn
	case types.IntType, types.IntPtrType:
		return arrow.PrimitiveTypes.Int64, intBuilderFn
	case types.Int8Type, types.Int8PtrType:
		return arrow.PrimitiveTypes.Int8, int8BuilderFn
	case types.Int16Type, types.Int16PtrType:
		return arrow.PrimitiveTypes.Int16, int16BuilderFn
	case types.Int32Type, types.Int32PtrType:
		return arrow.PrimitiveTypes.Int32, int32BuilderFn
	case types.Int64Type, types.Int64PtrType:
		return arrow.PrimitiveTypes.Int64, int64BuilderFn
	case types.UintType, types.UintPtrType:
		return arrow.PrimitiveTypes.Uint64, uintBuilderFn
	case types.Uint8Type, types.Uint8PtrType:
		return arrow.PrimitiveTypes.Uint8, uint8BuilderFn
	case types.Uint16Type, types.Uint16PtrType:
		return arrow.PrimitiveTypes.Uint16, uint16BuilderFn
	case types.Uint32Type, types.Uint32PtrType:
		return arrow.PrimitiveTypes.Uint32, uint32BuilderFn
	case types.Uint64Type, types.Uint64PtrType:
		return arrow.PrimitiveTypes.Uint64, uint64BuilderFn
	case types.StringType, types.StringPtrType:
		return arrow.BinaryTypes.String, stringBuilderFn
	case types.BytesType, types.BytesPtrType:
		return arrow.BinaryTypes.Binary, binaryBuilderFn
	case types.TimeType, types.TimePtrType:
		return arrow.BinaryTypes.String, timeBuilderFn
	case types.DateType, types.DatePtrType:
		return arrow.BinaryTypes.String, dateBuilderFn
	case types.ClockTimeType, types.ClockTimePtrType:
		return arrow.BinaryTypes.String, clockTimeBuilderFn
	case types.ClockTimeTZType, types.ClockTimeTZPtrType:
		return arrow.BinaryTypes.String, clockTimeTZBuilderFn
	case types.DateTimeType, types.DateTimePtrType:
		return arrow.BinaryTypes.String, dateTimeBuilderFn
	case types.TimestampType, types.TimestampPtrType:
		return arrow.BinaryTypes.String, timestampBuilderFn
	case types.NumericType, types.NumericPtrType:
		return arrow.BinaryTypes.String, numericBuilderFn
	case types.JSONType, types.JSONPtrType:
		return arrow.BinaryTypes.String, jsonBuilderFn
	case types.BinaryType, types.BinaryPtrType:
		return arrow.BinaryTypes.String, binaryStrBuilderFn
	}
	// NOTE: All unknown types are converted to strings. This could be
	// inefficient or incorrect in some cases. It's therefore best for the
	// database driver to always return the most appropriate scan type, if
	// possible.
	return arrow.BinaryTypes.String, unknownBuilderFn
}

// appender is a type capable of appending a specific type of value, or null,
// to an arrow column/array being built. All of the [array.Builder]
// implementations implement this generic interface for some type T.
type appender[T any] interface {
	Append(value T)
	AppendNull()
}

func basicBuilderFn[A appender[T], T any](builder array.Builder, value any) error {
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
		return &ArrowTypeError[T]{
			Actual: value,
		}
	}
	return nil
}

func convertBuilderFn[A appender[T], V any, T any](convert func(V) T) builderFn {
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
			return &ArrowTypeError[T]{
				Actual: val,
			}
		}
		return nil
	}
}

func timeToStr(value time.Time) string {
	return value.Format(time.RFC3339Nano)
}

type stringish interface {
	~string | ~[]byte
}

func castToStr[T stringish](value T) string {
	return string(value)
}

type int64ish interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

func castToInt64[T int64ish](value T) int64 {
	return int64(value)
}

type uint64ish interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

func castToUint64[T uint64ish](value T) uint64 {
	return uint64(value)
}

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
			out, err := json.Marshal(val)
			if err == nil {
				b.Append(string(out))
				return nil
			}
		}
		b.Append(fmt.Sprint(val))
	}
	return nil
}

// Marshal arrays, slices, and maps to JSON. Some driver implementations return
// these types (or custom types with these underlying types) for various
// object-like types. For example, ClickHouse returns orb.Pointer (whose
// underlying type is [2]float - see https://pkg.go.dev/github.com/paulmach/orb#Point)
// for the 'Point' data type. Note that this is not merely the scan type
// suggested by ColumnType.ScanType() - it's actually the type returned from
// the underlying driver, which means it cannot be scanned into a string even
// if we wanted, since database/sql doesn't know how to make the conversion.
// This is non-compliant with the database/sql package, as drivers are only
// supposed to return one of the driver.Value types (see:
// https://pkg.go.dev/database/sql/driver#Value). Ideally, this would be fixed
// upstream, so we could scan into a string and return the raw value. Until
// then, returning a JSON value is the next-best approach (without custom
// logic for each non-compliant type, which is another potential option).
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
