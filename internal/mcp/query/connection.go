package query

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect creates a connection pool for the PostgreSQL database. A pool
// (rather than a single connection) is used because MCP clients can issue
// concurrent tool calls, and *pgx.Conn is not safe for concurrent use. The
// pool also reestablishes connections transparently, so the server survives
// database restarts and idle-connection timeouts.
func Connect(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}

	// pgxpool connects lazily; ping so a bad connection string or unreachable
	// database fails at startup rather than on the first tool call.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
