package function

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
)

// Manager owns the function-tool state for the MCP server: one service
// entry per database whose @mcp functions have been introspected, and the
// set of registered tool names. All tool registration on the MCP server
// goes through the Manager so tool names stay collision-free.
type Manager struct {
	app    *common.App
	server *mcp.Server
	logger *slog.Logger
	// prefixTools controls whether tool names carry the database-name
	// prefix. The authoring server sets it, since it registers tools from
	// every database in the space alongside the built-in ghost_* tools; the
	// consumer serving mode exposes a single database's tools and nothing
	// else, so it registers unprefixed names.
	prefixTools bool

	// mu guards services and toolNames, and serializes refreshes (which
	// re-introspect and swap registered tools).
	mu       sync.Mutex
	services map[string]*service // database ID -> service
	// toolNames maps every registered function-tool name to the ID of the
	// database it belongs to.
	toolNames map[string]string
}

// service holds the live function-tool state for a single database.
type service struct {
	database  api.Database
	prefix    string
	pool      *pgxpool.Pool
	tools     []tool
	toolNames []string // currently-registered tool names for this service
}

// NewManager creates a Manager that registers function tools on server.
// prefixTools is described on the corresponding field.
func NewManager(app *common.App, server *mcp.Server, logger *slog.Logger, prefixTools bool) *Manager {
	return &Manager{
		app:         app,
		server:      server,
		logger:      logger,
		prefixTools: prefixTools,
		services:    map[string]*service{},
		toolNames:   map[string]string{},
	}
}

// LoadAll introspects the @mcp functions of every database in the space
// and registers the resulting tools. Databases that can't be introspected —
// paused, no stored password, unreachable — are skipped with a logged
// warning; databases with no @mcp functions are skipped silently.
//
// Building each database's service (connect, ping, introspect) runs
// concurrently, but naming and registration happen afterward in one
// single-threaded pass over listDatabases' (deterministically sorted)
// result, so a cross-database tool-name collision resolves the same way
// every time rather than depending on which database's connection happened
// to respond first.
//
// LoadAll is a startup-only snapshot: it assumes no databases are loaded
// yet, and unlike Load it never reloads an existing service.
func (m *Manager) LoadAll(ctx context.Context) {
	client, projectID, err := m.app.GetClient()
	if err != nil {
		m.logger.Warn("Skipping function tool registration (API client unavailable)",
			slog.Any("error", err),
		)
		return
	}

	databases, err := listDatabases(ctx, client, projectID)
	if err != nil {
		m.logger.Warn("Skipping function tool registration (failed to list databases)",
			slog.Any("error", err),
		)
		return
	}

	services := make([]*service, len(databases))
	var wg sync.WaitGroup
	for i, database := range databases {
		wg.Go(func() {
			if err := common.CheckReady(database); err != nil {
				m.logger.Debug("Skipping function tools for database that is not ready",
					slog.String("database", database.Name),
				)
				return
			}
			var prefix string
			if m.prefixTools {
				prefix = normalizeToolNameSegment(database.Name, "db")
			}
			svc, err := m.buildService(ctx, database, prefix)
			if err != nil {
				m.logger.Warn("Skipping function tools for database",
					slog.String("database", database.Name),
					slog.Any("error", err),
				)
				return
			}
			if len(svc.tools) == 0 {
				// Most databases never define function tools; don't hold an
				// idle pool open just to represent that.
				svc.pool.Close()
				return
			}
			services[i] = svc
		})
	}
	wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, svc := range services {
		if svc == nil {
			continue // not ready, unreachable, or no @mcp functions
		}
		m.services[svc.database.Id] = svc
		m.registerServiceTools(svc)
	}
}

// Load brings a single database's registered tools up to date and returns
// their names: it re-introspects the @mcp functions of an already-loaded
// database and swaps its tools (the MCP server emits tools/list_changed), or
// resolves, connects to, and registers a database it hasn't seen yet. Unlike
// LoadAll, any failure is returned — it backs the ghost_mcp_tool_refresh
// management tool, and the consumer serving mode's startup registration,
// which must abort when the served database can't be loaded.
func (m *Manager) Load(ctx context.Context, databaseRef string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, projectID, err := m.app.GetClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.GetDatabaseWithResponse(ctx, projectID, databaseRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}
	database := *resp.JSON200

	svc, ok := m.services[database.Id]
	if ok {
		// Already loaded: re-introspect and swap the registered tools.
		tools, err := introspect(ctx, m.logger.With(slog.String("database", svc.database.Name)), svc.pool)
		if err != nil {
			return nil, err
		}
		m.swapServiceTools(svc, tools)
	} else {
		// First load: connect, introspect, and register.
		if err := common.CheckReady(database); err != nil {
			return nil, err
		}

		var prefix string
		if m.prefixTools {
			prefix = normalizeToolNameSegment(database.Name, "db")
		}

		svc, err = m.buildService(ctx, database, prefix)
		if err != nil {
			return nil, err
		}

		m.services[database.Id] = svc
		m.registerServiceTools(svc)
	}

	// A database that ends up with no tools gets no cached service: there is
	// nothing for its connection to serve, so don't hold one open. A later
	// Load simply reconnects.
	if len(svc.toolNames) == 0 {
		delete(m.services, database.Id)
		svc.pool.Close()
	}

	return slices.Clone(svc.toolNames), nil
}

// IsFunctionTool reports whether name is a currently-registered generated
// function tool. Used by the analytics middleware to track all generated
// tool calls under a single event name (tool names are user-defined and
// unbounded).
func (m *Manager) IsFunctionTool(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.toolNames[name]
	return ok
}

// Close releases every service's connection pool.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, svc := range m.services {
		svc.pool.Close()
	}
	clear(m.services)
}

// buildService connects to the database and introspects its @mcp functions.
// The caller decides whether a service with no tools is worth keeping; both
// callers today close its pool and discard it.
func (m *Manager) buildService(ctx context.Context, database api.Database, prefix string) (*service, error) {
	const role = "tsdbadmin"

	password, err := common.GetPassword(database, role)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve password: %w", err)
	}

	// Function tools deliberately ignore Ghost's read_only config option:
	// marking a function @mcp is an intentional act, and the volatility-
	// derived annotations tell clients which tools write.
	connString, err := common.BuildConnectionString(common.ConnectionStringArgs{
		Database: database,
		Role:     role,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}

	// A pool (rather than a single connection) because MCP clients can issue
	// concurrent tool calls, and *pgx.Conn is not safe for concurrent use.
	// The pool also reestablishes connections transparently, so the server
	// survives database restarts and idle-connection timeouts.
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// pgxpool connects lazily; ping so a bad connection string or unreachable
	// database fails now rather than on the first tool call.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	tools, err := introspect(ctx, m.logger.With(slog.String("database", database.Name)), pool)
	if err != nil {
		pool.Close()
		return nil, err
	}

	return &service{
		database: database,
		prefix:   prefix,
		pool:     pool,
		tools:    tools,
	}, nil
}

// registerServiceTools registers the tools described by the service's
// current introspection. Each tool's name joins the database prefix
// (already normalized, empty in the consumer serving mode) and the
// normalized function name with toolNameSeparator; dedupeToolName resolves
// any collision, including same-named functions from different Postgres
// schemas within this service — schema is deliberately not part of the tool
// name (see the "Tool naming" section in CLAUDE.md), so that's the common
// case, not an edge case.
//
// Deterministic only if svc.tools is processed in the same order every
// time, which is introspect's job for one database (ORDER BY schema,
// function name); LoadAll handles the cross-database case.
//
// Callers must hold m.mu.
func (m *Manager) registerServiceTools(svc *service) {
	for _, tool := range svc.tools {
		var parts []string
		if svc.prefix != "" {
			parts = append(parts, svc.prefix)
		}
		parts = append(parts, normalizeToolNameSegment(tool.Name, "tool"))
		toolName := m.dedupeToolName(strings.Join(parts, toolNameSeparator))

		def, handler, err := buildMCPTool(toolName, tool, svc.pool)
		if err != nil {
			m.logger.Warn("Skipping @mcp function whose tool definition could not be built",
				slog.String("function", tool.Schema+"."+tool.Name),
				slog.String("database", svc.database.Name),
				slog.Any("error", err),
			)
			continue
		}
		m.server.AddTool(def, handler)
		m.toolNames[toolName] = svc.database.Id
		svc.toolNames = append(svc.toolNames, toolName)

		m.logger.Info("Registered function tool",
			slog.String("tool", toolName),
			slog.String("mode", string(tool.Mode)),
		)
	}
}

// swapServiceTools removes the service's currently-registered tools and
// registers the given tools instead. The MCP server emits
// tools/list_changed to connected sessions.
//
// Callers must hold m.mu.
func (m *Manager) swapServiceTools(svc *service, tools []tool) {
	if len(svc.toolNames) > 0 {
		m.server.RemoveTools(svc.toolNames...)
		for _, name := range svc.toolNames {
			delete(m.toolNames, name)
		}
	}
	svc.toolNames = nil
	svc.tools = tools

	m.registerServiceTools(svc)
}

// listDatabases retrieves every database in the space.
func listDatabases(ctx context.Context, client api.ClientWithResponsesInterface, projectID string) ([]api.Database, error) {
	resp, err := client.ListDatabasesWithResponse(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	databases := make([]api.Database, len(*resp.JSON200))
	for i, d := range *resp.JSON200 {
		databases[i] = api.Database{
			Host:       d.Host,
			Id:         d.Id,
			Name:       d.Name,
			Password:   d.Password,
			Port:       d.Port,
			Size:       d.Size,
			Status:     d.Status,
			StorageMib: d.StorageMib,
			Type:       d.Type,
		}
	}
	return databases, nil
}

// toolNameSeparator joins a tool name's segments (database prefix, function
// name), each already normalized by normalizeToolNameSegment. "_" is too
// ordinary a character within a normalized segment to reliably mark a
// boundary; "." was rejected even though the MCP spec allows it, because
// some real clients pass tool names straight through to their own model API
// without sanitizing them, and that validation can be narrower than the spec
// (e.g. the Claude API's tool-name pattern rejects a dot) — see
// isToolNameChar.
const toolNameSeparator = "__"

// isToolNameChar reports whether r is one of the characters this server
// allows in a composed tool name. Deliberately narrower than the MCP spec's
// own recommendation (which also allows "."), for the reason given on
// toolNameSeparator.
func isToolNameChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-'
}

// normalizeToolNameSegment converts a raw database or function name into a
// tool-name-safe segment: every run of characters outside isToolNameChar's
// set collapses to a single "_". Case is preserved. A segment with nothing
// legal in it at all (e.g. entirely emoji) falls back to fallback, which
// callers set to something identifying the segment kind (e.g. "db" or
// "tool") rather than an uninformative "_".
func normalizeToolNameSegment(name, fallback string) string {
	var b strings.Builder
	pendingSep := false
	for _, r := range name {
		if isToolNameChar(r) {
			if pendingSep && b.Len() > 0 {
				b.WriteByte('_')
			}
			pendingSep = false
			b.WriteRune(r)
		} else {
			pendingSep = true
		}
	}
	if b.Len() == 0 {
		return fallback
	}
	return b.String()
}

// maxToolNameLength is the MCP spec's recommended maximum tool name length,
// enforced here rather than left to the client — a client that submits its
// whole tool list on every request can fail the entire request over one
// oversized name, not just leave that tool unavailable. Checked as a byte
// count: normalizeToolNameSegment guarantees a composed name is always
// ASCII, so byte and rune counts agree.
const maxToolNameLength = 128

// dedupeToolName returns a variant of base that isn't already registered:
// base itself if free, otherwise base with "_2", "_3", ... appended, after
// truncating base to fit within maxToolNameLength (reserving room for the
// suffix). This never fails to find a name — a function only fails to
// become a tool if building its MCP tool definition itself errors (see
// registerServiceTools).
//
// The result is deterministic only if callers always resolve a given set of
// colliding names in the same order — see registerServiceTools and LoadAll.
//
// Callers must hold m.mu.
func (m *Manager) dedupeToolName(base string) string {
	if name := truncateToolName(base, ""); !m.toolNameTaken(name) {
		return name
	}
	for n := 2; ; n++ {
		if name := truncateToolName(base, fmt.Sprintf("_%d", n)); !m.toolNameTaken(name) {
			return name
		}
	}
}

// toolNameTaken reports whether name is already registered.
//
// Callers must hold m.mu.
func (m *Manager) toolNameTaken(name string) bool {
	_, taken := m.toolNames[name]
	return taken
}

// truncateToolName truncates base, if necessary, so that base+suffix fits
// within maxToolNameLength.
func truncateToolName(base, suffix string) string {
	if max := maxToolNameLength - len(suffix); len(base) > max {
		base = base[:max]
	}
	return base + suffix
}
