package dbdriver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/timescale/ghost/internal/serve/dbtypes"
)

const postgresClient = "postgres"

// ApplicationName is set in the Postgres connection's `application_name`
// runtime parameter so we are identifiable in `pg_stat_activity`.
var ApplicationName = "ghost-cli"

// OpenPostgresDSN opens a Postgres driver against the supplied DSN. The DSN
// should already include sslmode etc. (see common.BuildConnectionString).
func OpenPostgresDSN(ctx context.Context, dsn string) (Driver, error) {
	pgxCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing dsn: %w", err)
	}
	return openPostgresConfig(ctx, pgxCfg)
}

func openPostgresConfig(ctx context.Context, pgxCfg *pgx.ConnConfig) (d Driver, err error) {
	// Use pgx's default exec mode (extended protocol with a prepared-statement
	// cache). The widget splits multi-statement editor text into individual
	// statements for us, which we run one at a time, so we don't need the
	// simple protocol's multi-command support — and the extended protocol
	// avoids the simple protocol's downsides (client-side parameter
	// interpolation, no prepared-statement caching, weaker type handling).
	pgxCfg.RuntimeParams["application_name"] = ApplicationName

	tracer := &postgresQueryTracer{}
	pgxCfg.Tracer = tracer

	db := stdlib.OpenDB(*pgxCfg)
	defer closeDBOnErr(db, &err)

	base, err := newBaseDriver(ctx, postgresClient, db)
	if err != nil {
		return nil, err
	}
	defer closeConnOnErr(base.conn, &err)

	var pgConn *pgconn.PgConn
	if err := base.conn.Raw(func(driverConn any) error {
		pgConn = driverConn.(*stdlib.Conn).Conn().PgConn()
		return nil
	}); err != nil {
		return nil, fmt.Errorf("getting raw driver connection: %w", err)
	}

	return &postgresDriver{
		baseDriver:          base,
		postgresQueryTracer: tracer,
		pgConn:              pgConn,
	}, nil
}

// postgresQueryTracer captures the most recent pgconn.CommandTag so we can
// surface RowsAffected for INSERT/UPDATE/DELETE/etc., which database/sql
// hides behind a one-shot Result we can't get from a Query call.
type postgresQueryTracer struct {
	lastCommandTag *pgconn.CommandTag
}

func (t *postgresQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	t.lastCommandTag = nil
	return ctx
}

func (t *postgresQueryTracer) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	t.lastCommandTag = &data.CommandTag
}

type postgresDriver struct {
	baseDriver
	*postgresQueryTracer
	pgConn *pgconn.PgConn
}

// Context wraps the query in a cancellation handler that issues
// pg_cancel_backend() server-side when the parent context is canceled. This
// lets long-running queries terminate cleanly without dropping the
// connection mid-flight.
func (d *postgresDriver) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	return cancelContext(ctx, func(ctx context.Context) error {
		return d.pgConn.CancelRequest(ctx)
	})
}

func (d *postgresDriver) Query(ctx context.Context, query string) (Rows, error) {
	baseRows, err := d.query(ctx, query, d.scanType)
	if err != nil {
		return nil, err
	}
	return &postgresRows{
		baseRows:            *baseRows,
		postgresQueryTracer: d.postgresQueryTracer,
	}, nil
}

// scanType overlays Postgres-specific type targeting on top of baseDriver.
// JSON/JSONB go through our typed JSON scanner (preserves raw text);
// NUMERIC preserves arbitrary precision and special values (NaN, ±Inf);
// BYTEA goes through hex-encoding rather than raw bytes;
// DATE/TIMESTAMP/TIMESTAMPTZ use our string scanners that preserve the
// database's own formatting (the stdlib Postgres driver maps these to
// time.Time, which loses precision and special values like Infinity).
func (d *postgresDriver) scanType(columnType *sql.ColumnType) reflect.Type {
	switch columnType.DatabaseTypeName() {
	case "JSON", "JSONB":
		return dbtypes.JSONPtrType
	case "NUMERIC":
		return dbtypes.NumericPtrType
	case "BYTEA":
		return dbtypes.BinaryPtrType
	case "DATE":
		return dbtypes.DatePtrType
	case "TIMESTAMP":
		return dbtypes.DateTimePtrType
	case "TIMESTAMPTZ":
		return dbtypes.TimestampPtrType
	}
	return d.baseDriver.scanType(columnType)
}

func (d *postgresDriver) NormalizeError(ctx context.Context, err error) *NormalizedError {
	normalized := d.baseDriver.NormalizeError(ctx, err)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if strings.EqualFold(pgErr.Severity, "FATAL") {
			normalized.Fatal = true
		}
		// Translate the generic 42601 "syntax error" emitted by pgx when the
		// caller sends multiple statements to a single prepared call into our
		// own actionable message.
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

func (r *postgresRows) RowsAffected(_ context.Context) (*int64, error) {
	if r.lastCommandTag != nil {
		ra := r.lastCommandTag.RowsAffected()
		return &ra, nil
	}
	return nil, nil
}
