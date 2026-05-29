package dbdriver

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

	"github.com/timescale/ghost/internal/serve/dbtypes"
)

// Driver runs arbitrary SQL queries against a database connection. It wraps
// the underlying database/sql connection and adds OID-aware scan type
// inference, error normalization, and server-side cancellation hooks.
type Driver interface {
	Ping(ctx context.Context) error
	PingInterval() time.Duration

	// Context returns a context (not necessarily a child of ctx) that should
	// be passed to Query. CancelFunc must be invoked once the query completes.
	Context(ctx context.Context) (context.Context, context.CancelFunc)

	// Query issues a SQL statement and returns Rows. The context returned by
	// Context must be the one passed in.
	Query(ctx context.Context, args QueryArgs) (Rows, error)

	// NormalizeError adapts a database/sql or driver-specific error to the
	// wire NormalizedError shape expected by the widget.
	NormalizeError(ctx context.Context, err error) *NormalizedError

	Close() error
}

// QueryArgs is the input to Driver.Query.
type QueryArgs struct {
	Query      string
	ColumnCase ColumnCase
}

// baseDriver is the standard implementation, shared by every concrete driver.
// Driver-specific behavior (e.g. Postgres OID overrides, cancellation via
// pgconn.CancelRequest) is layered on top by embedding this type.
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

func (b *baseDriver) Query(ctx context.Context, args QueryArgs) (Rows, error) {
	return b.query(ctx, args, b.scanType)
}

func (b *baseDriver) query(ctx context.Context, args QueryArgs, scanTypeFn scanTypeFn) (*baseRows, error) {
	rows, err := b.conn.QueryContext(ctx, args.Query)
	if err != nil {
		return nil, err
	}
	return &baseRows{
		Rows:       rows,
		columnCase: args.ColumnCase,
		scanTypeFn: scanTypeFn,
	}, nil
}

// scanType maps a [sql.ColumnType] to the concrete Go type to scan into. The
// base implementation collapses sql.Null* into pointer-to-primitive (which
// JSON-encodes more cleanly) and ensures every scan target is addressable as
// a pointer so NULLs can be detected.
func (b *baseDriver) scanType(columnType *sql.ColumnType) reflect.Type {
	t := columnType.ScanType()
	switch t {
	case dbtypes.NullBoolType, dbtypes.NullBoolPtrType:
		t = dbtypes.BoolType
	case dbtypes.NullByteType, dbtypes.NullBytePtrType:
		t = dbtypes.ByteType
	case dbtypes.NullFloat64Type, dbtypes.NullFloat64PtrType:
		t = dbtypes.Float64Type
	case dbtypes.NullInt16Type, dbtypes.NullInt16PtrType:
		t = dbtypes.Int16Type
	case dbtypes.NullInt32Type, dbtypes.NullInt32PtrType:
		t = dbtypes.Int32Type
	case dbtypes.NullInt64Type, dbtypes.NullInt64PtrType:
		t = dbtypes.Int64Type
	case dbtypes.NullStringType, dbtypes.NullStringPtrType:
		t = dbtypes.StringType
	case dbtypes.NullTimeType, dbtypes.NullTimePtrType:
		t = dbtypes.TimeType
	case dbtypes.RawBytesType:
		t = dbtypes.BytesType
	case nil:
		t = dbtypes.AnyType
	}

	switch t.Kind() {
	case reflect.Pointer, reflect.Interface:
	default:
		t = reflect.PointerTo(t)
	}
	return t
}

func (b *baseDriver) NormalizeError(ctx context.Context, err error) *NormalizedError {
	ctxErr := context.Cause(ctx)
	return &NormalizedError{
		Message: b.errMessage(err),
		Source:  b.client,
		Fatal:   b.fatal(err),
		Timeout: errors.Is(ctxErr, context.DeadlineExceeded),
		Cancel:  errors.Is(ctxErr, context.Canceled),
	}
}

func (b *baseDriver) errMessage(err error) string {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "the database connection was terminated unexpectedly"
	}
	return err.Error()
}

var fatalErrs = []error{
	driver.ErrBadConn,
	sql.ErrConnDone,
	io.ErrUnexpectedEOF,
	net.ErrClosed,
}

func (b *baseDriver) fatal(err error) bool {
	for _, t := range fatalErrs {
		if errors.Is(err, t) {
			return true
		}
	}
	return b.invalidConn()
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
		errs = append(errs, fmt.Errorf("closing database connection: %w", err))
	}
	if err := b.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing database connection pool: %w", err))
	}
	return errors.Join(errs...)
}

func newBaseDriver(ctx context.Context, client string, db *sql.DB) (baseDriver, error) {
	db.SetMaxIdleConns(0)
	conn, err := db.Conn(ctx)
	if err != nil {
		return baseDriver{}, err
	}
	return baseDriver{client: client, db: db, conn: conn}, nil
}

func closeDBOnErr(db *sql.DB, err *error) {
	if err == nil || *err == nil {
		return
	}
	_ = db.Close()
}

func closeConnOnErr(conn *sql.Conn, err *error) {
	if err == nil || *err == nil {
		return
	}
	_ = conn.Close()
}
