package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/mcp/query"
)

// Generated query tools: every Ghost database can define a curated set of MCP
// tools as SQL queries (stored in the database itself; see internal/mcp/query).
// The authoring server registers the generated tools of every database in the
// space alongside the ghost_mcp_tool_* management tools; the stripped serving
// mode (Options.ServeQueryTools) exposes only one database's generated tools.

// queryToolsInstructions augments the authoring server's instructions when
// the query-tool feature is enabled.
const queryToolsInstructions = `

Custom query tools: each database can expose its own curated MCP tools, defined entirely as SQL queries stored in the database. A query tool runs one SQL query — its inputs are the query's bind parameters and its output is the returned row(s). Tools are validated against the live database with sqlc, so each tool's schema reflects the real parameter and column types, and are named with the snake_cased database name as a prefix (a query 'whatever' on database "My DB" becomes the tool 'my_db_whatever'). Use ghost_mcp_tool_create to define a new query tool (inspect the database first with ghost_schema), ghost_mcp_tool_get to read a tool's backing SQL, and ghost_mcp_tool_update / ghost_mcp_tool_delete to change or remove one. Changes take effect immediately.`

// serveInstructions are the instructions for the stripped consumer serving
// mode.
const serveInstructions = `This server exposes PostgreSQL queries as MCP tools. Each query tool runs one SQL query - its inputs are the query's bind parameters and its output is the returned row(s). Query tools are generated from SQL queries stored in the database and validated against the live database with sqlc, so each tool's schema reflects the real parameter and column types.`

// registerQueryTools sets up the query-tool manager on the authoring server
// and registers the management tools. When buildAll is set it also runs the
// startup snapshot, building and registering every database's generated
// query tools before the server starts serving (the per-database builds run
// concurrently, and databases that can't be built are skipped with a logged
// warning). Callers that only enumerate capabilities (e.g. `ghost mcp list`)
// leave buildAll unset, since enumerating must not connect to any databases.
func (s *Server) registerQueryTools(ctx context.Context, buildAll bool) {
	manager, err := query.NewManager(s.app, s.mcpServer, s.logger)
	if err != nil {
		s.logger.Error("Query tools unavailable",
			slog.Any("error", err),
		)
		return
	}
	s.queryManager = manager

	mcp.AddTool(s.mcpServer, newMCPToolCreateTool(), s.handleMCPToolCreate)
	mcp.AddTool(s.mcpServer, newMCPToolGetTool(), s.handleMCPToolGet)
	mcp.AddTool(s.mcpServer, newMCPToolUpdateTool(), s.handleMCPToolUpdate)
	mcp.AddTool(s.mcpServer, newMCPToolDeleteTool(), s.handleMCPToolDelete)

	if buildAll {
		manager.RegisterAll(ctx)
	}
}

// newQueryToolsServer creates a Server in the stripped consumer serving mode:
// it exposes only the given database's generated query tools — no management
// tools and no other Ghost tools. Unlike the authoring server, a database
// that can't be built is a fatal startup error, since its query tools are the
// entire tool surface being served.
func newQueryToolsServer(ctx context.Context, app *common.App, logger *slog.Logger, databaseRef string) (*Server, error) {
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Title:   serverTitle,
		Version: config.Version,
	}, &mcp.ServerOptions{
		Instructions: serveInstructions,
		Logger:       logger,
	})

	server := &Server{
		mcpServer: mcpServer,
		logger:    logger,
		app:       app,
	}

	manager, err := query.NewManager(app, mcpServer, logger)
	if err != nil {
		return nil, err
	}
	server.queryManager = manager

	if err := manager.RegisterServe(ctx, databaseRef); err != nil {
		return nil, fmt.Errorf("failed to build query tools for database %q: %w", databaseRef, err)
	}

	// Add analytics tracking middleware
	server.mcpServer.AddReceivingMiddleware(server.analyticsMiddleware)

	return server, nil
}

// Input property helpers shared by the ghost_mcp_tool_* management tools.

func queryToolNameInputProperties(schema *jsonschema.Schema) {
	schema.Properties["name"].Description = "Name of the query tool, without the database-name prefix (e.g. 'get_user', not 'my_db_get_user'). Only letters, digits, underscores, and hyphens are allowed."
}

func queryToolQueryInputProperties(schema *jsonschema.Schema) {
	schema.Properties["query"].Description = "The full SQL query, including the sqlc '-- name: <name> :<cmd>' directive (where <cmd> is one of :one, :many, or :exec) and any documentation comments. Comment lines after the directive become the tool's description."
}
