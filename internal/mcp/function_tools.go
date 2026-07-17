package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/mcp/function"
)

// Generated function tools: every Ghost database can define a curated set of
// MCP tools by marking Postgres functions with an @mcp comment (see
// internal/mcp/function). The authoring server (NewServer) registers the
// generated tools of every database in the space alongside the
// ghost_mcp_tool_refresh management tool; the stripped serving mode
// (NewFunctionToolsServer) exposes only one database's generated tools.

// functionToolsInstructions augments the authoring server's instructions
// when the function-tool feature is enabled.
const functionToolsInstructions = `

Custom function tools: each database can expose its own curated MCP tools, defined by marking Postgres functions with an @mcp comment (the first line of the COMMENT ON FUNCTION text is '@mcp'; the remaining lines become the tool's description). A function tool calls one function — its inputs are the function's arguments and its output is the returned row(s). Tool schemas are introspected from the database catalog, so they reflect the real argument and result types, and tools are named by joining the database name and the function name with a double underscore (a function 'whatever' on database 'billing' becomes the tool 'billing__whatever'). The function's schema plays no part in the name — a function 'whatever' in a 'reporting' schema becomes the same 'billing__whatever' (see below for what happens if that collides with another function of the same name). Each segment is normalized into a tool-safe name first (any run of characters other than letters, digits, underscores, and hyphens becomes a single underscore), so unusual database or function names still produce a usable tool rather than being skipped. Nothing is ever dropped for a naming reason: a name over 128 characters is truncated, and a name colliding with another tool's (e.g. the same function name used in two different schemas) gets a '_2', '_3', etc. suffix instead of being skipped. To add a capability, create the function and comment with ghost_sql, then call ghost_mcp_tool_refresh to pick up the change immediately.

Authoring rules for @mcp functions:
- Arguments with a DEFAULT are optional in the tool's input schema. All arguments reject null unless declared DEFAULT NULL, which makes an argument both optional and nullable.
- Declare read-only functions STABLE (or IMMUTABLE): the tool's read-only annotation comes from the function's volatility, and the default VOLATILE is treated as potentially writing.
- The declared return type determines the output: RETURNS <scalar or composite> and OUT parameters yield a single row, RETURNS SETOF/TABLE yields a list of rows, and RETURNS void yields a success acknowledgment (have the function return a count or summary if the caller needs one).
- Overloaded @mcp names (same name, different argument lists) each become their own tool, distinguished only by the '_2', '_3', etc. suffix described above. Prefer distinct function names when you want self-describing tools.
- A VARIADIC argument is exposed as an array parameter and passed through with the VARIADIC keyword. Give it a DEFAULT (e.g. '{}') to make it optional; otherwise the caller must supply the array (an empty array is allowed). A variadic function is called positionally, so any omitted defaults must be trailing.
- Unsupported and skipped with a logged warning: procedures (use a function returning void), polymorphic and VARIADIC "any" arguments, nested array types, aggregate and window functions, and RETURNS record without OUT parameters.`

// registerFunctionTools sets up the function-tool manager on the authoring
// server and registers the refresh management tool. When buildAll is set it
// also runs the startup snapshot, introspecting and registering every
// database's function tools before the server starts serving (the
// per-database introspections run concurrently, and databases that can't be
// reached are skipped with a logged warning). Callers that only enumerate
// capabilities (e.g. `ghost mcp list`) leave buildAll unset, since
// enumerating must not connect to any databases.
func (s *Server) registerFunctionTools(ctx context.Context, buildAll bool) {
	manager := function.NewManager(s.app, s.mcpServer, s.logger, true)
	s.functionManager = manager

	mcp.AddTool(s.mcpServer, newMCPToolRefreshTool(), s.handleMCPToolRefresh)

	if buildAll {
		manager.LoadAll(ctx)
	}
}

// NewFunctionToolsServer creates a Server in the stripped consumer serving mode:
// it exposes only the given database's generated function tools — no
// management tools and no other Ghost tools. This is the artifact you hand
// to someone as an API. Reached via `ghost mcp start --serve`. Unlike
// NewServer, a database that can't be introspected is a fatal startup error,
// since its function tools are the entire tool surface being served.
func NewFunctionToolsServer(ctx context.Context, app *common.App, logger *slog.Logger, databaseRef string) (*Server, error) {
	logger = ensureLogger(logger)

	// The serving mode deliberately sends no server instructions: how the
	// tools are implemented is irrelevant to a consumer, and the useful
	// content — what this particular tool surface is for — is something only
	// the database's author knows. TODO: let the author provide the
	// instructions from inside the database, e.g. via a designated database
	// or schema comment introspected alongside the @mcp functions.
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Title:   serverTitle,
		Version: config.Version,
	}, &mcp.ServerOptions{
		Logger: logger,
	})

	server := &Server{
		mcpServer: mcpServer,
		logger:    logger,
		app:       app,
	}

	manager := function.NewManager(app, mcpServer, logger, false)
	server.functionManager = manager

	if _, err := manager.Load(ctx, databaseRef); err != nil {
		return nil, fmt.Errorf("failed to build function tools for database %q: %w", databaseRef, err)
	}

	// Add analytics tracking middleware
	server.mcpServer.AddReceivingMiddleware(server.analyticsMiddleware)

	return server, nil
}
