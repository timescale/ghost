package driver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"time"

	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/types"
)

// Driver is a type capable of executing arbitrary SQL queries against a
// database instance. It wraps [sql.DB] and [sql.Conn] and provides additional
// methods that help to normalize the differences between various database/sql
// driver libraries.
type Driver interface {
	// Ping checks whether the underlying database connection is alive, and
	// returns an error if not. It is not safe to call this method concurrently
	// with itself or Query.
	Ping(ctx context.Context) error

	// PingInterval returns the interval of time that should pass between Ping
	// calls, for this database type. Some databases benefit from more frequent
	// pings than others.
	PingInterval() time.Duration

	// Context returns a new context that should be passed to Query. The
	// returned context will not necessarily be a child of the passed-in
	// context, and may differ with respect to cancellation and timeout
	// behavior. The cancel function should be called after the query completes
	// to free resources.
	Context(ctx context.Context) (context.Context, context.CancelFunc)

	// Query issues an SQL query and returns the results. For cancellation and
	// timeouts to work correctly, the passed-in context should be the result
	// of a call to Context. It is not safe to call this method concurrently
	// with itself or Ping.
	Query(ctx context.Context, query string) (Rows, error)

	// NormalizeError normalizes errors returned from different sql/database
	// driver libraries. Normalized errors also carry information about whether
	// the query was canceled, timed out, or executed on a broken database
	// connection.
	NormalizeError(ctx context.Context, err error) *api.NormalizedError

	// Close waits for all in-progress queries to finish before closing the
	// underlying database connection/pool.
	Close() error
}

// baseDriver is the standard implementation of the [Driver] interface. Most
// sql/database driver libraries work fine with just this baseDriver
// implementation. However, in order to customize things like query
// cancellation behavior and handling of unusual data types, some driver
// implementations embed this type and override/extend its functionality.
type baseDriver struct {
	client string
	db     *sql.DB
	conn   *sql.Conn
}

func (b *baseDriver) Ping(ctx context.Context) error {
	return b.conn.PingContext(ctx)
}

func (b *baseDriver) PingInterval() time.Duration {
	return 5 * time.Second
}

func (b *baseDriver) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctx, func() {}
}

func (b *baseDriver) Query(ctx context.Context, query string) (Rows, error) {
	return b.query(ctx, query, b.scanType)
}

func (b *baseDriver) query(ctx context.Context, query string, scanTypeFn scanTypeFn) (*baseRows, error) {
	rows, err := b.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}

	return &baseRows{
		Rows:       rows,
		scanTypeFn: scanTypeFn,
	}, nil
}

func (b *baseDriver) scanType(columnType *sql.ColumnType) reflect.Type {
	t := columnType.ScanType()
	switch t {
	// NOTE: Some drivers (e.g. trino) return sql.NullWhatever (or
	// *sql.NullWhatever) types, which don't serialize to JSON well. A pointer
	// to a built-in type works just as well for scanning nullable values.
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
		// Some non-compliant drivers (e.g. mssql) will sometimes return nil
		// for the scan type, instead of any. Fix that here.
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

func (b *baseDriver) NormalizeError(ctx context.Context, err error) *api.NormalizedError {
	ctxErr := context.Cause(ctx)
	return &api.NormalizedError{
		Message: b.errMessage(err),
		Source:  b.client,
		Fatal:   b.fatal(err),
		Timeout: errors.Is(ctxErr, context.DeadlineExceeded),
		Cancel:  errors.Is(ctxErr, context.Canceled),
	}
}

// errMessage returns more user-friendly error messages for some low-level
// errors (such as [io.ErrUnexpectedEOF]), for use in [NormalizedError].
func (b *baseDriver) errMessage(err error) string {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "the database connection was terminated unexpectedly"
	}
	return err.Error()
}

// Standard error types that indicate a broken/invalid database connection.
// Driver-specific error types are handled in the driver's implementation of
// the [Driver.NormalizeError] method.
var fatalErrs = []error{
	driver.ErrBadConn,
	sql.ErrConnDone,
	io.ErrUnexpectedEOF,
	net.ErrClosed,
}

// fatal returns true if the provided err indicates a broken/invalid database
// connection, or if the underlying driver for the [sql.Conn] returns false
// from [driver.Validator.IsValid].
func (b *baseDriver) fatal(err error) bool {
	return errorIsAny(err, fatalErrs...) || b.invalidConn()
}

func errorIsAny(err error, targets ...error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}

func (b *baseDriver) invalidConn() bool {
	var invalid bool
	if err := b.conn.Raw(func(driverConn any) error {
		if v, ok := driverConn.(driver.Validator); ok {
			invalid = !v.IsValid()
		}
		return nil
	}); errors.Is(err, driver.ErrBadConn) {
		return true
	}
	return invalid
}

func (b *baseDriver) Close() error {
	var errs []error
	if err := b.conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
		errs = append(errs, fmt.Errorf("error closing database connection: %w", err))
	}
	if err := b.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("error closing database connection pool: %w", err))
	}
	return errors.Join(errs...)
}
