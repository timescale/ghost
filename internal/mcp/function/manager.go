package function

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
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
	// prefixTools controls whether tool names carry the snake_cased
	// database-name prefix. The authoring server sets it, since it registers
	// tools from every database in the space alongside the built-in ghost_*
	// tools; the consumer serving mode exposes a single database's tools and
	// nothing else, so it registers bare function names.
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
	tools     []Tool
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
// and registers the resulting tools, running the per-database introspection
// concurrently. Databases that can't be introspected — paused, no stored
// password, unreachable — are skipped with a logged warning; their tools
// simply don't appear until a refresh or restart when they're available.
// Databases with no @mcp functions are skipped silently (and their
// connections closed).
//
// LoadAll is a startup-only snapshot: it assumes no databases are loaded
// yet, and unlike Load it never reloads an existing service. If a
// refresh-everything operation is ever needed (e.g. picking up other space
// members' changes without a restart), rework this to share Load's
// load-or-reload semantics rather than calling it twice.
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
	prefixes := m.computePrefixes(databases)

	var wg sync.WaitGroup
	for _, database := range databases {
		wg.Go(func() {
			if err := common.CheckReady(database); err != nil {
				m.logger.Debug("Skipping function tools for database that is not ready",
					slog.String("database", database.Name),
				)
				return
			}
			svc, err := m.buildService(ctx, database, prefixes[database.Id])
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

			m.mu.Lock()
			defer m.mu.Unlock()
			m.services[database.Id] = svc
			m.registerServiceTools(svc)
		})
	}
	wg.Wait()
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
		tools, err := Introspect(ctx, m.logger.With(slog.String("database", svc.database.Name)), svc.pool)
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
			prefix = m.assignPrefix(database)
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

	tools, err := Introspect(ctx, m.logger.With(slog.String("database", database.Name)), pool)
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
// current introspection. Each function name is qualified with its own
// Postgres schema (unless "public") and then the service's database prefix
// (empty in the consumer serving mode, which registers bare function/schema
// names). A name that can't form a legal tool name, or that is already
// taken by another service's tool, is skipped with a warning.
//
// Callers must hold m.mu.
func (m *Manager) registerServiceTools(svc *service) {
	for _, tool := range svc.tools {
		if !toolNamePattern.MatchString(tool.Name) {
			m.logger.Warn("Skipping @mcp function whose name cannot form a tool name",
				slog.String("function", tool.Schema+"."+tool.Name),
				slog.String("database", svc.database.Name),
			)
			continue
		}

		name := tool.Name
		if tool.Schema != "public" {
			if !toolNamePattern.MatchString(tool.Schema) {
				m.logger.Warn("Skipping @mcp function whose schema cannot form a tool name",
					slog.String("function", tool.Schema+"."+tool.Name),
					slog.String("database", svc.database.Name),
				)
				continue
			}
			name = tool.Schema + "_" + name
		}

		toolName := name
		if svc.prefix != "" {
			toolName = svc.prefix + "_" + name
		}
		if _, taken := m.toolNames[toolName]; taken {
			m.logger.Warn("Skipping function tool with duplicate name",
				slog.String("tool", toolName),
				slog.String("database", svc.database.Name),
			)
			continue
		}

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
func (m *Manager) swapServiceTools(svc *service, tools []Tool) {
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

// computePrefixes assigns each database in a startup snapshot a unique
// tool-name prefix, comparing every database against the same one-time
// listing. See assignPrefix for prefix assignment outside that snapshot
// (e.g. a database registered after startup), which must not rely on a
// second listing being ordered the same way as the first.
func (m *Manager) computePrefixes(databases []api.Database) map[string]string {
	taken := make(map[string]bool, len(databases))
	prefixes := make(map[string]string, len(databases))

	for _, database := range databases {
		prefixes[database.Id] = m.assignPrefixFrom(database, taken)
	}

	return prefixes
}

// assignPrefix computes the tool prefix for a database that's being
// registered outside the startup snapshot (a refresh, or the consumer
// serving mode's initial load). Unlike the startup snapshot, this doesn't
// re-list every database in the space: the prefixes already assigned to
// live services (the only ones that can actually collide) are the source of
// truth.
//
// Callers must hold m.mu.
func (m *Manager) assignPrefix(database api.Database) string {
	taken := make(map[string]bool, len(m.services))
	for _, svc := range m.services {
		taken[svc.prefix] = true
	}
	return m.assignPrefixFrom(database, taken)
}

// assignPrefixFrom computes database's tool prefix given the prefixes
// already taken by other databases, logging when disambiguation was
// necessary. See nextPrefix for the disambiguation rules.
func (m *Manager) assignPrefixFrom(database api.Database, taken map[string]bool) string {
	prefix := nextPrefix(database.Name, database.Id, taken)
	if prefix != toolPrefix(database.Name) {
		m.logger.Warn("Database name produces a conflicting tool prefix; disambiguating with ID suffix",
			slog.String("database", database.Name),
			slog.String("prefix", prefix),
		)
	}
	return prefix
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
