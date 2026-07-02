package query

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Classification records whether a query writes to the database, as
// determined from its EXPLAIN plan. It backs the read-only/destructive
// annotations on the query tools.
type Classification struct {
	// ReadOnly is true when the query's plan contains no ModifyTable node,
	// i.e. it performs no INSERT/UPDATE/DELETE/MERGE. A read-only plan can
	// still invoke a function that writes; like all MCP tool annotations,
	// this is a hint, not a guarantee.
	ReadOnly bool

	// Destructive is true when the query can modify or remove existing rows.
	// Plain INSERTs (without ON CONFLICT ... DO UPDATE) are additive, so they
	// are writes but not destructive.
	Destructive bool
}

// classifyTimeout bounds each EXPLAIN round-trip during classification.
const classifyTimeout = 5 * time.Second

// ClassifyQueries determines, for each query, whether it writes to the
// database by asking PostgreSQL to EXPLAIN it. EXPLAIN only plans the query
// (it does not execute it), so this is cheap and has no side effects. The
// queries have already been validated by sqlc against the same database, so
// planning them with all-NULL parameters is expected to succeed.
//
// Classification is best-effort: if EXPLAIN fails for a query (e.g. it is a
// utility statement, which EXPLAIN does not support), its Classification is
// left nil and the corresponding tool carries no read-only/destructive hints.
func ClassifyQueries(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, queries []Query) {
	for i := range queries {
		c, err := classifyQuery(ctx, pool, queries[i])
		if err != nil {
			logger.Warn("Could not classify query (its tool will carry no read-only hint)",
				slog.String("query", queries[i].Name),
				slog.Any("error", err),
			)
			continue
		}
		queries[i].Classification = c
	}
}

func classifyQuery(ctx context.Context, pool *pgxpool.Pool, query Query) (*Classification, error) {
	ctx, cancel := context.WithTimeout(ctx, classifyTimeout)
	defer cancel()

	// Bind NULL for every parameter; EXPLAIN plans the statement without
	// running it, so the values only need to type-check.
	args := make([]any, len(query.Params))

	// EXPLAIN's json output scans directly into the destination via pgx's
	// JSON codec.
	var plans []struct {
		Plan map[string]any `json:"Plan"`
	}
	if err := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+query.Text, args...).Scan(&plans); err != nil {
		return nil, err
	}

	c := &Classification{ReadOnly: true}
	for _, plan := range plans {
		scanPlanForWrites(plan.Plan, c)
	}

	return c, nil
}

// scanPlanForWrites walks a plan tree (the "Plan" object of EXPLAIN's JSON
// output) and records every ModifyTable node in c. Writes can appear below
// the root, e.g. data-modifying CTEs, so the whole tree is searched.
func scanPlanForWrites(node map[string]any, c *Classification) {
	if node == nil {
		return
	}

	if nodeType, _ := node["Node Type"].(string); nodeType == "ModifyTable" {
		c.ReadOnly = false

		// An INSERT is additive unless an ON CONFLICT clause lets it update
		// existing rows; UPDATE/DELETE/MERGE are destructive.
		op, _ := node["Operation"].(string)
		conflict, _ := node["Conflict Resolution"].(string)
		if op != "Insert" || conflict == "UPDATE" {
			c.Destructive = true
		}
	}

	children, _ := node["Plans"].([]any)
	for _, child := range children {
		if childNode, ok := child.(map[string]any); ok {
			scanPlanForWrites(childNode, c)
		}
	}
}
