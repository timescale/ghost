package driver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/timescale/ghost/internal/serve/api"
	"github.com/timescale/ghost/internal/serve/types"
)

const postgresDriverName = "pgx"

type postgresAdapter struct {
	baseAdapter[*postgresDriver]
}

func newPostgresAdapter(client string) *postgresAdapter {
	return &postgresAdapter{
		baseAdapter: newBaseAdapter(postgresDriverName, client, newPostgresDriver),
	}
}

func (a *postgresAdapter) open(ctx context.Context, dsn string) (d *postgresDriver, err error) {
	pgxCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	return a.openPgxConfig(ctx, pgxCfg)
}

func (a *postgresAdapter) openPgxConfig(ctx context.Context, pgxCfg *pgx.ConnConfig) (d *postgresDriver, err error) {
	pgxCfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	pgxCfg.RuntimeParams["application_name"] = applicationName

	tracer := &postgresQueryTracer{}
	pgxCfg.Tracer = tracer

	// Install our query-cancellation handler (see [queryCanceler]). It needs
	// the *sql.DB to issue pg_cancel_backend, but the handler is built during
	// the first connection (inside OpenDB), so the DB is injected into the
	// canceler immediately afterward — before any connection is opened.
	canceler := &queryCanceler{}
	pgxCfg.BuildContextWatcherHandler = canceler.newHandler

	db := stdlib.OpenDB(*pgxCfg)
	defer closeDBOnErr(ctx, db, &err)
	canceler.db = db

	driver, err := a.newDriver(ctx, db)
	if err != nil {
		return nil, err
	}
	driver.postgresQueryTracer = tracer
	return driver, nil
}

// Implements the [pgx.QueryTracer] interface. Exists solely for the purpose of
// getting access to the [pgconn.CommandTag], which contains the number of rows
// affected (for INSERT/UPDATE/DELETE statements). Getting the number of rows
// affected is not possible via the database/sql Query methods (and we can't
// use the Exec methods, because we don't know for for sure whether the user's
// queries will return rows or not).
type postgresQueryTracer struct {
	lastCommandTag *pgconn.CommandTag
}

func (t *postgresQueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	t.lastCommandTag = nil
	return ctx
}

func (t *postgresQueryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	t.lastCommandTag = &data.CommandTag
}

// cancelTimeout bounds the pg_cancel_backend exchange so a stalled network
// can't hang the cancellation goroutine indefinitely.
const cancelTimeout = 10 * time.Second

// queryCanceler builds the per-connection [ctxwatch.Handler] that pgx invokes
// when a query's context is canceled. It holds the *sql.DB used to open the
// (separate) connection that issues the cancel; the DB is injected after
// stdlib.OpenDB (see openPgxConfig).
type queryCanceler struct {
	db *sql.DB
}

func (c *queryCanceler) newHandler(pgConn *pgconn.PgConn) ctxwatch.Handler {
	return &cancelHandler{
		db:     c.db,
		pgConn: pgConn,
	}
}

// cancelHandler cancels the in-progress query on pgConn's backend when the
// query's context is canceled.
//
// Neither of pgx's built-in handlers works for us. The default cancels by
// closing the underlying connection, which would tear down the session — not
// acceptable here. The other, [pgconn.CancelRequestContextWatcherHandler],
// sends the native Postgres cancel ([pgconn.PgConn.CancelRequest]) over a fresh
// connection, but does not negotiate TLS the way the original connection (and
// libpq) does, so the cancel goes out as a plaintext packet:
// https://github.com/jackc/pgx/issues/2340. Ghost databases sit behind a
// TLS/SNI-routing proxy, and with no TLS handshake there's no SNI for it to
// route on — so the proxy drops the cancel and it never reaches the backend.
//
// Instead we issue pg_cancel_backend over a normal connection. The backend's
// own connection is busy running the query, so this opens a fresh one — with
// the full dial + TLS + SNI + auth handled by pgx — to the same backend, then
// cancels the query by PID. This is heavier than a native cancel request (a
// full authenticated connection vs. a lightweight cancel packet), but cancels
// are rare, interactive actions, so the overhead is immaterial. If/when the pgx
// issue above is resolved so cancel requests negotiate TLS, we can drop this
// and use [pgconn.CancelRequestContextWatcherHandler] instead.
//
// NOTE: this relies on the connection pool being allowed to open a second
// connection while the query connection is in use (i.e. MaxOpenConns must not
// be 1), otherwise this would deadlock against the running query.
type cancelHandler struct {
	db     *sql.DB
	pgConn *pgconn.PgConn
	done   chan struct{}
}

// HandleCancel is called by pgx when the query's context is canceled.
func (h *cancelHandler) HandleCancel(context.Context) {
	h.done = make(chan struct{})
	go func() {
		defer close(h.done)

		ctx, cancel := context.WithTimeout(context.Background(), cancelTimeout)
		defer cancel()

		if _, err := h.db.ExecContext(ctx, "SELECT pg_cancel_backend($1)", h.pgConn.PID()); err != nil {
			// We couldn't cancel server-side; break the connection so the query
			// doesn't block until the run times out.
			_ = h.pgConn.Conn().SetDeadline(time.Now())
		}
	}()
}

// HandleUnwatchAfterCancel is called by pgx once the canceled query has
// returned. Waiting for the cancel to finish before the connection can be
// reused ensures a late pg_cancel_backend can't cancel the next query on this
// backend.
func (h *cancelHandler) HandleUnwatchAfterCancel() {
	<-h.done
	_ = h.pgConn.Conn().SetDeadline(time.Time{})
}

type postgresDriver struct {
	baseDriver
	*postgresQueryTracer
	pgConn *pgconn.PgConn
}

func newPostgresDriver(ctx context.Context, b baseDriver) (d *postgresDriver, err error) {
	var pgConn *pgconn.PgConn
	if err := b.conn.Raw(func(driverConn any) error {
		pgConn = driverConn.(*stdlib.Conn).Conn().PgConn()
		return nil
	}); err != nil {
		return nil, fmt.Errorf("error getting raw driver connection: %w", err)
	}

	return &postgresDriver{
		baseDriver: b,
		pgConn:     pgConn,
	}, nil
}

func (d *postgresDriver) Query(ctx context.Context, query string) (Rows, error) {
	baseRows, err := d.query(ctx, query, d.scanType)
	if err != nil {
		return nil, err
	}

	return &postgresRows{
		baseRows:            *baseRows,
		postgresQueryTracer: d.postgresQueryTracer,
	}, err
}

func (d *postgresDriver) scanType(columnType *sql.ColumnType) reflect.Type {
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
	return d.baseDriver.scanType(columnType)
}

func (d *postgresDriver) NormalizeError(ctx context.Context, err error) *api.NormalizedError {
	normalized := d.baseDriver.NormalizeError(ctx, err)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if strings.EqualFold(pgErr.Severity, "FATAL") {
			normalized.Fatal = true
		}

		// Return a more user-friendly error if the user attempts to run
		// multiple statements in a single query. It's possible we will support
		// this in the future, but it's a little complicated - see #20.
		// NOTE: Error code 42601 is just the generic "syntax error" code, so
		// we also need to check the message.
		if pgErr.Code == "42601" && pgErr.Message == "cannot insert multiple commands into a prepared statement" {
			normalized.Message = ErrMultiStatement.Error()
			return normalized
		}

		normalized.Code = pgErr.Code
		normalized.Detail = pgErr.Detail
		normalized.Hint = pgErr.Hint
		normalized.Message = pgErr.Message
		normalized.Position = pgErr.Position
	}
	return normalized
}

type postgresRows struct {
	baseRows
	*postgresQueryTracer
}

func (r *postgresRows) RowsAffected(ctx context.Context) (*int64, error) {
	if r.lastCommandTag != nil {
		rowsAffected := r.lastCommandTag.RowsAffected()
		return &rowsAffected, nil
	}
	return nil, nil
}
