package query

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timescale/ghost/internal/common"
)

// Builder produces query metadata by running sqlc over a set of stored
// queries. The same Builder is used once at startup and again whenever a
// stored query changes, so each Build call manages (and cleans up) its own
// temporary files.
type Builder struct {
	pool    *pgxpool.Pool
	exePath string
	logger  *slog.Logger
}

// NewBuilder creates a Builder. pool provides the database connection (its
// original connection string is reused for the sqlc config), exePath is the
// path to this executable (used as the sqlc plugin command), and logger
// receives progress and warning output.
func NewBuilder(pool *pgxpool.Pool, exePath string, logger *slog.Logger) *Builder {
	return &Builder{
		pool:    pool,
		exePath: exePath,
		logger:  logger,
	}
}

// Build runs the full sqlc pipeline against the given stored queries and
// returns the resulting query metadata. An empty query set short-circuits to
// empty metadata, since sqlc treats a query file with no queries as an error
// (and there is nothing to build anyway).
func (b *Builder) Build(ctx context.Context, stored []StoredQuery) (*QueryMetadata, error) {
	if len(stored) == 0 {
		return &QueryMetadata{}, nil
	}

	// sqlc connects to the database itself, using the same connection string
	// the pool was created with.
	connString := b.pool.Config().ConnConfig.ConnString()

	// Every file sqlc needs (the schema DDL, the query file, its config, and
	// the query metadata it produces) lives in a single scratch directory
	// under the OS temp location.
	tmpDir, err := os.MkdirTemp("", "ghost-mcp-queries-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			b.logger.Warn("Failed to remove scratch directory",
				slog.String("dir", tmpDir),
				slog.Any("error", err),
			)
		}
	}()

	// sqlc needs schema information to resolve parameter names and
	// nullability, which the wire protocol alone cannot provide. Generate
	// schema DDL from Ghost's own schema introspection (avoiding a pg_dump
	// dependency) and give sqlc both the schema file
	// and the database connection (hybrid analysis). Definitions are included
	// so function bodies make it into the DDL. The default schema filters
	// exclude Ghost's reserved schema, so the stored-queries table itself
	// never appears in the DDL.
	b.logger.Debug("Generating schema DDL...")
	conn, err := b.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire database connection: %w", err)
	}
	schemas, err := common.FetchSchemaObjects(ctx, conn.Conn(), common.SchemaObjectsArgs{
		IncludeDefinitions: true,
	})
	conn.Release()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch database schema: %w", err)
	}
	ddl := common.FormatSchemaDDL(schemas)
	if err := os.WriteFile(filepath.Join(tmpDir, "schema.sql"), []byte(ddl), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write schema file: %w", err)
	}

	// Join the stored query blocks into a single sqlc query file.
	if err := os.WriteFile(filepath.Join(tmpDir, "queries.sql"), JoinQueryBlocks(stored), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write query file: %w", err)
	}

	b.logger.Debug("Generating sqlc config file...")
	sqlcConfigPath := filepath.Join(tmpDir, "sqlc.yaml")
	if err := GenerateSqlcConfig(sqlcConfigPath, SqlcConfig{
		SchemaPath:  "schema.sql",
		QueriesPath: "queries.sql",
		OutPath:     ".",
		PluginCmd:   b.exePath,
		DatabaseURL: connString,
	}); err != nil {
		return nil, fmt.Errorf("failed to generate sqlc config file: %w", err)
	}

	b.logger.Debug("Running sqlc to process queries...")
	if err := RunSqlc(ctx, sqlcConfigPath); err != nil {
		return nil, fmt.Errorf("failed to run sqlc: %w", err)
	}

	b.logger.Debug("Loading query metadata...")
	meta, err := LoadMetadata(filepath.Join(tmpDir, "queries.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	// Ask the database (via EXPLAIN) whether each query writes, so the MCP
	// tools can carry read-only/destructive annotations. Classification is
	// best-effort: a query that cannot be classified simply gets no hints.
	ClassifyQueries(ctx, b.logger, b.pool, meta.Queries)

	return meta, nil
}
