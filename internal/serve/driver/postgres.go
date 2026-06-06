package driver

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

	db := stdlib.OpenDB(*pgxCfg)
	defer closeDBOnErr(ctx, db, &err)

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
