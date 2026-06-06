package driver

import (
	"context"
	"database/sql"
	"io"
	"log/slog"

	"github.com/timescale/ghost/internal/log"
)

// Supported database client types.
const (
	Postgres  = "postgresql"
	Timescale = "timescale"
)

var applicationName string

// SetApplicationName sets the global application name that is used by some
// adapters (e.g. the postgres adapter) to identify the application connecting
// to the database.
func SetApplicationName(name string) {
	applicationName = name
}

// Open creates a new [Driver] instance for the specified client type, using
// the provided DSN. If the client type is invalid, it returns
// [InvalidClientTypeError].
func Open(ctx context.Context, client, dsn string) (Driver, error) {
	a, exists := adapters[client]
	if !exists {
		return nil, &InvalidClientTypeError{
			Client: client,
		}
	}
	return a.open(ctx, dsn)
}

// adapters is a lookup from client type to the [adapter] implementation
// responsible for creating concrete [Driver] instances for that client type.
// NOTE: This has to be wrapped in a function for generic type inference for
// newAdapterWrapper to work correctly, for some reason.
var adapters = func() map[string]adapter[Driver] {
	return map[string]adapter[Driver]{
		Postgres:  newAdapterWrapper(newPostgresAdapter(Postgres)),
		Timescale: newAdapterWrapper(newPostgresAdapter(Timescale)),
	}
}()

// adapter is an internal interface for a type capable of creating a new
// [Driver] instance for a particular database client type, given a DSN string.
type adapter[D Driver] interface {
	open(ctx context.Context, dsn string) (D, error)
}

// adapterWrapper is a thin wrapper around a concrete adapter type. It
// implements adapter[Driver] (with the interface type as the generic type
// argument), converting the returned concrete driver type to the [Driver]
// interface type.
type adapterWrapper[D Driver] struct {
	adapter[D]
}

func newAdapterWrapper[D Driver](a adapter[D]) *adapterWrapper[D] {
	return &adapterWrapper[D]{
		adapter: a,
	}
}

func (c *adapterWrapper[D]) open(ctx context.Context, dsn string) (Driver, error) {
	driver, err := c.adapter.open(ctx, dsn)
	if err != nil {
		// Ensures we don't return a non-nil Driver interface that points to a
		// nil concrete implementation.
		return nil, err
	}
	return driver, nil
}

// newDriverFn is a function that is capable of returning a new concrete
// [Driver] implementation, given a [baseDriver].
type newDriverFn[D Driver] func(ctx context.Context, b baseDriver) (D, error)

// baseAdapter is a basic implementation of the adapter interface. It is
// typically embedded in a more specific adapter type to provide some baseline
// functionality shared between adapters, but it can also be used as-is.
type baseAdapter[D Driver] struct {
	driver      string
	client      string
	newDriverFn newDriverFn[D]
}

func newBaseAdapter[D Driver](driver, client string, newDriverFn newDriverFn[D]) baseAdapter[D] {
	return baseAdapter[D]{
		driver:      driver,
		client:      client,
		newDriverFn: newDriverFn,
	}
}

func (a *baseAdapter[D]) open(ctx context.Context, dsn string) (d D, err error) {
	db, err := sql.Open(a.driver, dsn)
	if err != nil {
		return d, err
	}
	defer closeDBOnErr(ctx, db, &err)

	return a.newDriver(ctx, db)
}

func (a *baseAdapter[D]) newDriver(ctx context.Context, db *sql.DB) (d D, err error) {
	b, err := a.newBaseDriver(ctx, db)
	if err != nil {
		return d, err
	}
	defer closeConnOnErr(ctx, b.conn, &err)

	return a.newDriverFn(ctx, b)
}

func (a *baseAdapter[D]) newBaseDriver(ctx context.Context, db *sql.DB) (baseDriver, error) {
	db.SetMaxIdleConns(0)

	conn, err := db.Conn(ctx)
	if err != nil {
		return baseDriver{}, err
	}

	return baseDriver{
		client: a.client,
		db:     db,
		conn:   conn,
	}, nil
}

func closeOnErr(closer io.Closer, err *error) error {
	if err == nil || *err == nil {
		return nil
	}
	return closer.Close()
}

func closeDBOnErr(ctx context.Context, db *sql.DB, err *error) {
	if err := closeOnErr(db, err); err != nil {
		logger := log.FromContext(ctx)
		logger.Error("Error closing database connection pool", slog.Any("error", err))
	}
}

func closeConnOnErr(ctx context.Context, conn *sql.Conn, err *error) {
	if err := closeOnErr(conn, err); err != nil {
		logger := log.FromContext(ctx)
		logger.Error("Error closing database connection", slog.Any("error", err))
	}
}
