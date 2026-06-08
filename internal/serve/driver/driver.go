package driver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve/api"
)

// applicationName identifies this application to the database via the
// application_name connection parameter.
const applicationName = "ghost"

// Source is the value reported in the Source field of an [api.NormalizedError]
// for errors originating from this driver. The driver only supports Postgres,
// so this is constant.
const Source = "postgresql"

// Driver executes arbitrary SQL queries against a Postgres database instance.
// It wraps [sql.DB] and [sql.Conn] and provides additional methods for
// normalizing query results and errors.
type Driver struct {
	db     *sql.DB
	conn   *sql.Conn
	pgConn *pgconn.PgConn
	tracer *postgresQueryTracer
}

// Open establishes a new Postgres [Driver] connection using the provided DSN.
// It opens the connection pool, acquires a single dedicated connection, and
// wires up the query tracer and cancellation handler. If an error occurs after
// a resource is acquired, that resource is closed before returning.
func Open(ctx context.Context, dsn string) (d *Driver, err error) {
	pgxCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	pgxCfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	pgxCfg.RuntimeParams["application_name"] = applicationName

	tracer := &postgresQueryTracer{}
	pgxCfg.Tracer = tracer

	// Install our query-cancellation handler (see [cancelHandler]). It needs
	// the *sql.DB to issue pg_cancel_backend, but the handler is built during
	// the first connection (inside OpenDB), so the DB is injected into the
	// canceler immediately afterward — before any connection is opened.
	canceler := &queryCanceler{}
	pgxCfg.BuildContextWatcherHandler = canceler.newHandler

	db := stdlib.OpenDB(*pgxCfg)
	defer closeDBOnErr(ctx, db, &err)
	canceler.db = db

	db.SetMaxIdleConns(0)

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer closeConnOnErr(ctx, conn, &err)

	var pgConn *pgconn.PgConn
	if err := conn.Raw(func(driverConn any) error {
		pgConn = driverConn.(*stdlib.Conn).Conn().PgConn()
		return nil
	}); err != nil {
		return nil, fmt.Errorf("error getting raw driver connection: %w", err)
	}

	return &Driver{
		db:     db,
		conn:   conn,
		pgConn: pgConn,
		tracer: tracer,
	}, nil
}

// Ping checks whether the underlying database connection is alive, returning an
// error if not. It is not safe to call concurrently with itself or Query.
func (d *Driver) Ping(ctx context.Context) error {
	return d.conn.PingContext(ctx)
}

// Query issues an SQL query and returns the results. Canceling the passed-in
// context cancels the in-progress query via the connection's context-watcher
// handler. It is not safe to call concurrently with itself or Ping.
func (d *Driver) Query(ctx context.Context, query string) (*Rows, error) {
	rows, err := d.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}

	return &Rows{
		Rows:   rows,
		tracer: d.tracer,
	}, nil
}

// NormalizeError normalizes an error returned from the database driver. The
// returned error also carries information about whether the query was canceled,
// timed out, or executed on a broken database connection.
func (d *Driver) NormalizeError(ctx context.Context, err error) *api.NormalizedError {
	ctxErr := context.Cause(ctx)
	normalized := &api.NormalizedError{
		Message: errMessage(err),
		Source:  Source,
		Fatal:   fatal(err),
		Timeout: errors.Is(ctxErr, context.DeadlineExceeded),
		Cancel:  errors.Is(ctxErr, context.Canceled),
	}

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

// errMessage returns more user-friendly error messages for some low-level
// errors (such as [io.ErrUnexpectedEOF]), for use in [api.NormalizedError].
func errMessage(err error) string {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "the database connection was terminated unexpectedly"
	}
	return err.Error()
}

// Standard error types that indicate a broken/invalid database connection.
var fatalErrs = []error{
	driver.ErrBadConn,
	sql.ErrConnDone,
	io.ErrUnexpectedEOF,
	net.ErrClosed,
}

// fatal returns true if the provided err indicates a broken/invalid database
// connection.
func fatal(err error) bool {
	return slices.ContainsFunc(fatalErrs, func(target error) bool {
		return errors.Is(err, target)
	})
}

// Close waits for all in-progress queries to finish before closing the
// underlying database connection and pool.
func (d *Driver) Close() error {
	var errs []error
	if err := d.conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
		errs = append(errs, fmt.Errorf("error closing database connection: %w", err))
	}
	if err := d.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("error closing database connection pool: %w", err))
	}
	return errors.Join(errs...)
}

// postgresQueryTracer implements the [pgx.QueryTracer] interface. It exists
// solely for the purpose of getting access to the [pgconn.CommandTag], which
// contains the number of rows affected (for INSERT/UPDATE/DELETE statements).
// Getting the number of rows affected is not possible via the database/sql
// Query methods (and we can't use the Exec methods, because we don't know for
// sure whether the user's queries will return rows or not).
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
// stdlib.OpenDB (see Open).
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

func closeOnErr(closer io.Closer, err *error) error {
	if err == nil || *err == nil {
		return nil
	}
	return closer.Close()
}
