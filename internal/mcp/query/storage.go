package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timescale/ghost/internal/common"
)

// Query tool definitions live in a reserved schema inside the service's own
// database, one row per query tool. Storing them in the database (rather than
// in local files or Ghost API storage) means they are shared across the space,
// fork and share along with the data, and die with the service. The schema and
// table are created lazily on the first tool create, so databases that never
// define query tools are untouched.

// StoredQuery is a single query tool definition stored in the service's
// database. SQL holds the full sqlc query block, including the
// `-- name: <name> :<cmd>` directive and any documentation comments.
type StoredQuery struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
}

// storageTable is the fully-qualified table holding the stored queries.
const storageTable = common.ReservedSchema + ".mcp_queries"

// EnsureStorage creates the reserved schema and queries table if they do not
// exist yet. Called lazily before the first write; reads treat a missing table
// as an empty query set instead.
func EnsureStorage(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+common.ReservedSchema); err != nil {
		return fmt.Errorf("failed to create reserved schema: %w", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+storageTable+` (
    name text PRIMARY KEY,
    sql text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("failed to create queries table: %w", err)
	}
	return nil
}

// LoadStoredQueries returns every stored query, ordered by name. A database
// where the storage table has never been created simply has no stored
// queries, so that case returns an empty result rather than an error.
func LoadStoredQueries(ctx context.Context, pool *pgxpool.Pool) ([]StoredQuery, error) {
	rows, err := pool.Query(ctx, "SELECT name, sql FROM "+storageTable+" ORDER BY name")
	if err != nil {
		if isMissingStorage(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read stored queries: %w", err)
	}
	stored, err := pgx.CollectRows(rows, pgx.RowToStructByPos[StoredQuery])
	if err != nil {
		if isMissingStorage(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read stored queries: %w", err)
	}
	return stored, nil
}

// InsertStoredQuery adds a new stored query.
func InsertStoredQuery(ctx context.Context, pool *pgxpool.Pool, q StoredQuery) error {
	if _, err := pool.Exec(ctx,
		"INSERT INTO "+storageTable+" (name, sql) VALUES ($1, $2)",
		q.Name, q.SQL,
	); err != nil {
		return fmt.Errorf("failed to store query %q: %w", q.Name, err)
	}
	return nil
}

// UpdateStoredQuery replaces the SQL of an existing stored query.
func UpdateStoredQuery(ctx context.Context, pool *pgxpool.Pool, q StoredQuery) error {
	tag, err := pool.Exec(ctx,
		"UPDATE "+storageTable+" SET sql = $2, updated_at = now() WHERE name = $1",
		q.Name, q.SQL,
	)
	if err != nil {
		return fmt.Errorf("failed to update stored query %q: %w", q.Name, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no stored query named %q", q.Name)
	}
	return nil
}

// DeleteStoredQuery removes a stored query by name.
func DeleteStoredQuery(ctx context.Context, pool *pgxpool.Pool, name string) error {
	tag, err := pool.Exec(ctx,
		"DELETE FROM "+storageTable+" WHERE name = $1",
		name,
	)
	if err != nil {
		if isMissingStorage(err) {
			return fmt.Errorf("no stored query named %q", name)
		}
		return fmt.Errorf("failed to delete stored query %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no stored query named %q", name)
	}
	return nil
}

// isMissingStorage reports whether err indicates the reserved schema or the
// queries table does not exist (it is created lazily on the first write).
func isMissingStorage(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	// 42P01 = undefined_table, 3F000 = invalid_schema_name
	return pgErr.Code == "42P01" || pgErr.Code == "3F000"
}
