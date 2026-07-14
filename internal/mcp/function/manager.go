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
func NewManager(app *common.App, server *mcp.Server, logger *slog.Logger) *Manager {
	return &Manager{
		app:       app,
		server:    server,
		logger:    logger,
		services:  map[string]*service{},
		toolNames: map[string]string{},
	}
}

// RegisterAll introspects the @mcp functions of every database in the space
// and registers the resulting tools, running the per-database introspection
// concurrently. Databases that can't be introspected — paused, no stored
// password, unreachable — are skipped with a logged warning; their tools
// simply don't appear until a refresh or restart when they're available.
// Databases with no @mcp functions are skipped silently (and their
// connections closed).
func (m *Manager) RegisterAll(ctx context.Context) {
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
			svc, err := m.buildService(ctx, database, prefixes[database.Id], false)
			if err != nil {
				m.logger.Warn("Skipping function tools for database",
					slog.String("database", database.Name),
					slog.Any("error", err),
				)
				return
			}
			if svc == nil {
				// No @mcp functions.
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

// RegisterServe introspects and registers the function tools for a single
// database. This is the consumer serving mode: unlike RegisterAll, any
// failure is returned so the caller can abort startup — the function tools
// are the entire tool surface being served. The tools use the same
// database-name prefix they would have in the authoring server, so a tool's
// name is identical everywhere it appears.
func (m *Manager) RegisterServe(ctx context.Context, databaseRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.ensureService(ctx, databaseRef)
	return err
}

// Refresh re-introspects a database's @mcp functions and swaps its
// registered tools, picking up functions created, changed, or dropped since
// the last introspection (the MCP server emits tools/list_changed). It
// returns the names of the database's currently-registered tools.
func (m *Manager) Refresh(ctx context.Context, databaseRef string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	svc, err := m.ensureService(ctx, databaseRef)
	if err != nil {
		return nil, err
	}

	logger := m.logger.With(
		slog.String("database", svc.database.Name),
	)
	tools, err := Introspect(ctx, logger, svc.pool)
	if err != nil {
		return nil, err
	}

	m.swapServiceTools(svc, tools)
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
// When keepEmpty is false and the database has no @mcp functions, the
// connection is closed and (nil, nil) is returned — most databases never
// define function tools, and this keeps the startup snapshot from holding
// idle pools open for them.
func (m *Manager) buildService(ctx context.Context, database api.Database, prefix string, keepEmpty bool) (*service, error) {
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

	pool, err := Connect(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	tools, err := Introspect(ctx, m.logger.With(slog.String("database", database.Name)), pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if len(tools) == 0 && !keepEmpty {
		pool.Close()
		return nil, nil
	}

	return &service{
		database: database,
		prefix:   prefix,
		pool:     pool,
		tools:    tools,
	}, nil
}

// ensureService returns the service for the given database ref,
// creating (and caching) a connection for it if this is the first operation
// to target it.
//
// Callers must hold m.mu.
func (m *Manager) ensureService(ctx context.Context, databaseRef string) (*service, error) {
	client, projectID, err := m.app.GetClient()
	if err != nil {
		return nil, err
	}

	database, err := fetchDatabase(ctx, client, projectID, databaseRef)
	if err != nil {
		return nil, err
	}
	if svc, ok := m.services[database.Id]; ok {
		return svc, nil
	}
	if err := common.CheckReady(database); err != nil {
		return nil, err
	}

	prefix, err := m.databasePrefix(ctx, client, projectID, database.Id)
	if err != nil {
		return nil, err
	}

	svc, err := m.buildService(ctx, database, prefix, true)
	if err != nil {
		return nil, err
	}

	m.services[database.Id] = svc
	m.registerServiceTools(svc)
	return svc, nil
}

// registerServiceTools registers the tools described by the service's
// current introspection, prefixing each function name with the service's
// tool prefix. A name that can't form a legal tool name, or that is already
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
		toolName := svc.prefix + "_" + tool.Name
		if _, taken := m.toolNames[toolName]; taken {
			m.logger.Warn("Skipping function tool with duplicate name",
				slog.String("tool", toolName),
				slog.String("database", svc.database.Name),
			)
			continue
		}

		def, handler := buildMCPTool(toolName, tool, svc.pool)
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

// computePrefixes assigns each database a unique tool-name prefix derived
// from its snake_cased name. Database names are unique within a space, so two
// databases can only produce the same prefix when their names differ solely
// by case or separator style; when that happens, the first database in the
// list keeps the plain prefix and each later one is disambiguated with a
// short suffix derived from its ID, with a warning logged. Prefixes that
// would land in the built-in ghost_* tool namespace are disambiguated the
// same way.
func (m *Manager) computePrefixes(databases []api.Database) map[string]string {
	taken := make(map[string]bool, len(databases))
	prefixes := make(map[string]string, len(databases))

	for _, database := range databases {
		prefix := toolPrefix(database.Name)
		if taken[prefix] || prefix == "ghost" || strings.HasPrefix(prefix, "ghost_") {
			disambiguated := disambiguatePrefix(prefix, database.Id, taken)
			m.logger.Warn("Database name produces a conflicting tool prefix; disambiguating with ID suffix",
				slog.String("database", database.Name),
				slog.String("prefix", disambiguated),
			)
			prefix = disambiguated
		}
		taken[prefix] = true
		prefixes[database.Id] = prefix
	}

	return prefixes
}

// databasePrefix computes the tool prefix for a single database against the
// full current database list, so a database targeted directly (serve mode or
// a refresh of a database that wasn't registered at startup) gets the same
// prefix RegisterAll would have assigned it.
func (m *Manager) databasePrefix(ctx context.Context, client api.ClientWithResponsesInterface, projectID, databaseID string) (string, error) {
	databases, err := listDatabases(ctx, client, projectID)
	if err != nil {
		return "", err
	}
	prefixes := m.computePrefixes(databases)
	prefix, ok := prefixes[databaseID]
	if !ok {
		return "", fmt.Errorf("database %q not found in space", databaseID)
	}
	return prefix, nil
}

// fetchDatabase retrieves the database details from the API by ref (name or
// ID).
func fetchDatabase(ctx context.Context, client api.ClientWithResponsesInterface, projectID, databaseRef string) (api.Database, error) {
	resp, err := client.GetDatabaseWithResponse(ctx, projectID, databaseRef)
	if err != nil {
		return api.Database{}, fmt.Errorf("failed to get database: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return api.Database{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return api.Database{}, errors.New("empty response from API")
	}
	return *resp.JSON200, nil
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
