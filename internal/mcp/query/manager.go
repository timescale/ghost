package query

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
)

// Manager owns the query-tool state for the MCP server: one service entry per
// database whose stored queries have been built, the set of registered tool
// names, and the management operations that create, read, update, and delete
// stored queries. All tool registration on the MCP server goes through the
// Manager so tool names stay collision-free.
type Manager struct {
	app     *common.App
	server  *mcp.Server
	logger  *slog.Logger
	exePath string

	// mu guards services and toolNames, and serializes management operations
	// (which rebuild metadata and swap registered tools).
	mu       sync.Mutex
	services map[string]*service // database ID -> service
	// toolNames maps every registered query-tool name to the ID of the
	// database it belongs to.
	toolNames map[string]string
}

// service holds the live query-tool state for a single database.
type service struct {
	database  api.Database
	prefix    string
	pool      *pgxpool.Pool
	builder   *Builder
	metadata  *QueryMetadata
	toolNames []string // currently-registered tool names for this service
}

// NewManager creates a Manager that registers query tools on server.
func NewManager(app *common.App, server *mcp.Server, logger *slog.Logger) (*Manager, error) {
	// The path to this executable doubles as the sqlc plugin command (the
	// ghost binary re-enters itself in plugin mode; see RunPlugin).
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	return &Manager{
		app:       app,
		server:    server,
		logger:    logger,
		exePath:   exePath,
		services:  make(map[string]*service),
		toolNames: make(map[string]string),
	}, nil
}

// RegisterAll builds query metadata for every database in the space and
// registers the resulting query tools, running the per-service builds
// concurrently. Databases that can't be built — paused, no stored password,
// unreachable — are skipped with a logged warning; their tools simply don't
// appear until a later restart when they're available. Databases with no
// stored queries are skipped silently (and their connections closed).
//
// This is the authoring server's startup snapshot: edits made through this
// server instance are picked up live, edits made elsewhere on the next
// restart.
func (m *Manager) RegisterAll(ctx context.Context) {
	client, projectID, err := m.app.GetClient()
	if err != nil {
		m.logger.Warn("Skipping query tool registration (API client unavailable)",
			slog.Any("error", err),
		)
		return
	}

	databases, err := listDatabases(ctx, client, projectID)
	if err != nil {
		m.logger.Warn("Skipping query tool registration (failed to list databases)",
			slog.Any("error", err),
		)
		return
	}
	prefixes := m.computePrefixes(databases)

	var wg sync.WaitGroup
	for _, database := range databases {
		wg.Go(func() {
			if err := common.CheckReady(database); err != nil {
				m.logger.Debug("Skipping query tools for database that is not ready",
					slog.String("database", database.Name),
				)
				return
			}
			svc, err := m.buildService(ctx, database, prefixes[database.Id], false)
			if err != nil {
				m.logger.Warn("Skipping query tools for database",
					slog.String("database", database.Name),
					slog.Any("error", err),
				)
				return
			}
			if svc == nil {
				// No stored queries.
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

// RegisterServe builds and registers the query tools for a single database.
// This is the consumer serving mode: unlike RegisterAll, any failure is
// returned so the caller can abort startup — the query tools are the entire
// tool surface being served. The tools use the same database-name prefix they
// would have in the authoring server, so a tool's name is identical
// everywhere it appears.
func (m *Manager) RegisterServe(ctx context.Context, databaseRef string) error {
	client, projectID, err := m.app.GetClient()
	if err != nil {
		return err
	}

	database, err := fetchDatabase(ctx, client, projectID, databaseRef)
	if err != nil {
		return err
	}
	if err := common.CheckReady(database); err != nil {
		return err
	}

	prefix, err := m.databasePrefix(ctx, client, projectID, database.Id)
	if err != nil {
		return err
	}

	svc, err := m.buildService(ctx, database, prefix, true)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.services[database.Id] = svc
	m.registerServiceTools(svc)
	return nil
}

// IsQueryTool reports whether name is a currently-registered generated query
// tool. Used by the analytics middleware to track all generated tool calls
// under a single event name (tool names are user-defined and unbounded).
func (m *Manager) IsQueryTool(name string) bool {
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
	m.services = make(map[string]*service)
}

// --- Management operations ---
//
// Create and Update validate the new SQL by running the full sqlc build over
// the would-be query set before anything is persisted; an invalid query is
// rejected with sqlc's diagnostics and the stored queries are left untouched.
// On success the stored queries are updated and the service's registered
// tools are swapped immediately (emitting tools/list_changed).

// CreateTool validates and stores a new query tool for the given database.
func (m *Manager) CreateTool(ctx context.Context, databaseRef, name, query string) error {
	if err := ValidateQueryBlock(name, query); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	svc, err := m.ensureServiceLocked(ctx, databaseRef)
	if err != nil {
		return err
	}

	stored, err := LoadStoredQueries(ctx, svc.pool)
	if err != nil {
		return err
	}
	if _, ok := findStoredQuery(stored, name); ok {
		return fmt.Errorf("query tool %q already exists; use ghost_mcp_tool_update to modify it", name)
	}

	newSet := append(stored, StoredQuery{Name: name, SQL: query})
	meta, err := svc.builder.Build(ctx, newSet)
	if err != nil {
		return fmt.Errorf("query rejected: %w", err)
	}

	if err := EnsureStorage(ctx, svc.pool); err != nil {
		return err
	}
	if err := InsertStoredQuery(ctx, svc.pool, StoredQuery{Name: name, SQL: query}); err != nil {
		return err
	}

	m.swapServiceTools(svc, meta)
	return nil
}

// GetTool returns the stored SQL (including its sqlc directive and
// documentation comments) backing a query tool.
func (m *Manager) GetTool(ctx context.Context, databaseRef, name string) (StoredQuery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	svc, err := m.ensureServiceLocked(ctx, databaseRef)
	if err != nil {
		return StoredQuery{}, err
	}

	stored, err := LoadStoredQueries(ctx, svc.pool)
	if err != nil {
		return StoredQuery{}, err
	}
	q, ok := findStoredQuery(stored, name)
	if !ok {
		return StoredQuery{}, fmt.Errorf("no query tool named %q", name)
	}
	return q, nil
}

// UpdateTool validates and replaces the SQL backing an existing query tool.
func (m *Manager) UpdateTool(ctx context.Context, databaseRef, name, query string) error {
	if err := ValidateQueryBlock(name, query); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	svc, err := m.ensureServiceLocked(ctx, databaseRef)
	if err != nil {
		return err
	}

	stored, err := LoadStoredQueries(ctx, svc.pool)
	if err != nil {
		return err
	}
	i, ok := findStoredQueryIndex(stored, name)
	if !ok {
		return fmt.Errorf("no query tool named %q; use ghost_mcp_tool_create to create it", name)
	}

	stored[i].SQL = query
	meta, err := svc.builder.Build(ctx, stored)
	if err != nil {
		return fmt.Errorf("query rejected: %w", err)
	}

	if err := UpdateStoredQuery(ctx, svc.pool, stored[i]); err != nil {
		return err
	}

	m.swapServiceTools(svc, meta)
	return nil
}

// DeleteTool removes an existing query tool and its stored SQL.
func (m *Manager) DeleteTool(ctx context.Context, databaseRef, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	svc, err := m.ensureServiceLocked(ctx, databaseRef)
	if err != nil {
		return err
	}

	stored, err := LoadStoredQueries(ctx, svc.pool)
	if err != nil {
		return err
	}
	i, ok := findStoredQueryIndex(stored, name)
	if !ok {
		return fmt.Errorf("no query tool named %q", name)
	}

	// Rebuild the metadata without the removed query so the remaining tools
	// swap in cleanly.
	remaining := append(stored[:i:i], stored[i+1:]...)
	meta, err := svc.builder.Build(ctx, remaining)
	if err != nil {
		return err
	}

	if err := DeleteStoredQuery(ctx, svc.pool, name); err != nil {
		return err
	}

	m.swapServiceTools(svc, meta)
	return nil
}

// --- Internal helpers ---

// buildService connects to the database and builds its query metadata. When
// keepEmpty is false and the database has no stored queries, the connection
// is closed and (nil, nil) is returned — most databases never define query
// tools, and this keeps the startup snapshot from holding idle pools open for
// them.
func (m *Manager) buildService(ctx context.Context, database api.Database, prefix string, keepEmpty bool) (*service, error) {
	const role = "tsdbadmin"

	password, err := common.GetPassword(database, role)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve password: %w", err)
	}

	// Query tools deliberately ignore Ghost's read_only config option:
	// creating a query tool is an intentional act, and the EXPLAIN-derived
	// annotations tell clients which tools write.
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

	stored, err := LoadStoredQueries(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if len(stored) == 0 && !keepEmpty {
		pool.Close()
		return nil, nil
	}

	builder := NewBuilder(pool, m.exePath, m.logger.With(slog.String("database", database.Name)))
	meta, err := builder.Build(ctx, stored)
	if err != nil {
		pool.Close()
		return nil, err
	}

	return &service{
		database: database,
		prefix:   prefix,
		pool:     pool,
		builder:  builder,
		metadata: meta,
	}, nil
}

// ensureServiceLocked returns the service for the given database ref,
// creating (and caching) a connection for it if this is the first management
// operation to target it. Newly-created services start with empty metadata;
// the management operation that needed the service performs its own build.
//
// Callers must hold m.mu.
func (m *Manager) ensureServiceLocked(ctx context.Context, databaseRef string) (*service, error) {
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

// registerServiceTools registers the tools described by the service's current
// metadata, prefixing each query name with the service's tool prefix. A name
// that is already taken by another service's tool is skipped with a warning.
//
// Callers must hold m.mu.
func (m *Manager) registerServiceTools(svc *service) {
	for _, query := range svc.metadata.Queries {
		toolName := svc.prefix + "_" + query.Name
		if _, taken := m.toolNames[toolName]; taken {
			m.logger.Warn("Skipping query tool with duplicate name",
				slog.String("tool", toolName),
				slog.String("database", svc.database.Name),
			)
			continue
		}

		tool, handler := BuildQueryTool(toolName, query, svc.metadata, svc.pool)
		m.server.AddTool(tool, handler)
		m.toolNames[toolName] = svc.database.Id
		svc.toolNames = append(svc.toolNames, toolName)

		m.logger.Info("Registered query tool",
			slog.String("tool", toolName),
			slog.String("cmd", query.Cmd),
		)
	}
}

// swapServiceTools removes the service's currently-registered query tools and
// registers the tools described by meta. The MCP server emits
// tools/list_changed to connected sessions.
//
// Callers must hold m.mu.
func (m *Manager) swapServiceTools(svc *service, meta *QueryMetadata) {
	if len(svc.toolNames) > 0 {
		m.server.RemoveTools(svc.toolNames...)
		for _, name := range svc.toolNames {
			delete(m.toolNames, name)
		}
	}
	svc.toolNames = nil
	svc.metadata = meta

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
// a management operation on a database that wasn't registered at startup)
// gets the same prefix RegisterAll would have assigned it.
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

func findStoredQuery(stored []StoredQuery, name string) (StoredQuery, bool) {
	if i, ok := findStoredQueryIndex(stored, name); ok {
		return stored[i], true
	}
	return StoredQuery{}, false
}

func findStoredQueryIndex(stored []StoredQuery, name string) (int, bool) {
	for i, q := range stored {
		if q.Name == name {
			return i, true
		}
	}
	return 0, false
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
