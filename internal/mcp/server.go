package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/analytics"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
)

const (
	ServerName  = "ghost"
	serverTitle = "Ghost"
)

// Server wraps the MCP server with Ghost-specific functionality
type Server struct {
	mcpServer       *mcp.Server
	docsProxyClient *ProxyClient
	logger          *slog.Logger
	app             *common.App
	// browser drives an in-process web UI for the visualize/chart/ui_state
	// tools. Non-nil only in local (stdio) mode, where opening a browser makes
	// sense; nil for the remote HTTP transport.
	browser *browserController
}

// Options configures optional [Server] behavior.
type Options struct {
	// Local indicates the server is running in local (stdio) mode, where it can
	// open a browser on the user's machine. Enables the visualize/chart/
	// ui_state tools backed by an in-process web UI.
	Local bool
}

// NewServer creates a new Ghost MCP server instance
func NewServer(ctx context.Context, app *common.App, logger *slog.Logger) (*Server, error) {
	return NewServerWithOptions(ctx, app, logger, Options{})
}

// NewServerWithOptions creates a new Ghost MCP server instance with the given
// [Options].
func NewServerWithOptions(ctx context.Context, app *common.App, logger *slog.Logger, opts Options) (*Server, error) {
	logger = ensureLogger(logger)
	instructions := "Ghost provides tools for creating, managing, and querying fully-managed PostgreSQL databases. " +
		"Use it to provision new databases, fork existing ones for isolation and testing migrations, share database copies with other users, pause and resume instances, execute SQL queries, inspect schemas, and manage credentials. " +
		"It also provides access to PostgreSQL, TimescaleDB, and PostGIS documentation through semantic and keyword search, " +
		"plus skills with best-practice guidance for working with Postgres: schema and table design (data types, indexing, constraints, JSONB, partitioning), TimescaleDB hypertables for time-series data, pgvector embeddings for semantic search and RAG, hybrid BM25 + vector search, and PostGIS spatial data. " +
		"Consult these skills when designing schemas or setting up Postgres features like time-series, vector, or full-text search. " +
		"A free monthly compute allowance is included (shared across your space; databases auto-pause when it's reached), so creating and forking databases for experimentation is low-risk."

	// Append a directive to the instructions when an update is available, so
	// the agent proactively surfaces the outdated CLI to the user. Runs
	// synchronously so the message is included in the MCP server's
	// initialization response. Errors are logged but never block server
	// startup.
	if cfg := app.GetConfig(); cfg.VersionCheck {
		res, err := common.CheckVersion(ctx, cfg.ReleasesURL)
		if err != nil {
			logger.Error("version check failed", slog.String("error", err.Error()))
		} else if res.UpdateAvailable {
			instructions += fmt.Sprintf(
				"\n\nIMPORTANT: The user's Ghost CLI is outdated (running %s; latest is %s). "+
					"Before performing any Ghost-related work, proactively inform the user that their "+
					"Ghost CLI is outdated and recommend they upgrade by running: %s. "+
					"After upgrading, the user will need to reconnect the Ghost MCP server for the new version to take effect.",
				res.CurrentVersion, res.LatestVersion, res.UpdateCommand,
			)
		}
	}

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Title:   serverTitle,
		Version: config.Version,
	}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       logger,
	})

	server := &Server{
		mcpServer: mcpServer,
		logger:    logger,
		app:       app,
	}
	if opts.Local {
		server.browser = newBrowserController(app, logger)
	}

	// Register all tools (including proxied docs tools)
	server.registerTools(ctx)

	// Add analytics tracking middleware
	server.mcpServer.AddReceivingMiddleware(server.analyticsMiddleware)

	return server, nil
}

func ensureLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.DiscardHandler)
}

// StartStdio starts the MCP server with the stdio transport
func (s *Server) StartStdio(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

// Returns an HTTP handler that implements the http transport
func (s *Server) HTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return s.mcpServer
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
	})
}

// registerTools registers all available MCP tools
func (s *Server) registerTools(ctx context.Context) {
	// Register remote docs MCP server proxy
	s.registerDocsProxy(ctx)

	// Register authentication tools
	mcp.AddTool(s.mcpServer, newLoginTool(), s.handleLogin)

	// Register Ghost database tools
	mcp.AddTool(s.mcpServer, newIDTool(), s.handleID)
	mcp.AddTool(s.mcpServer, newUsageTool(), s.handleUsage)
	mcp.AddTool(s.mcpServer, newListTool(), s.handleList)
	mcp.AddTool(s.mcpServer, newCreateTool(), s.handleCreate)
	mcp.AddTool(s.mcpServer, newDeleteTool(), s.handleDelete)
	mcp.AddTool(s.mcpServer, newForkTool(), s.handleFork)
	mcp.AddTool(s.mcpServer, newPauseTool(), s.handlePause)
	mcp.AddTool(s.mcpServer, newResumeTool(), s.handleResume)
	mcp.AddTool(s.mcpServer, newConnectTool(), s.handleConnect)
	mcp.AddTool(s.mcpServer, newSQLTool(), s.handleSQL)
	mcp.AddTool(s.mcpServer, newSchemaTool(), s.handleSchema)
	mcp.AddTool(s.mcpServer, newPasswordTool(), s.handlePassword)
	mcp.AddTool(s.mcpServer, newLogsTool(), s.handleLogs)
	mcp.AddTool(s.mcpServer, newFeedbackTool(), s.handleFeedback)
	mcp.AddTool(s.mcpServer, newRenameTool(), s.handleRename)
	mcp.AddTool(s.mcpServer, newCreateDedicatedTool(), s.handleCreateDedicated)
	mcp.AddTool(s.mcpServer, newForkDedicatedTool(), s.handleForkDedicated)
	mcp.AddTool(s.mcpServer, newShareTool(), s.handleShare)
	mcp.AddTool(s.mcpServer, newShareListTool(), s.handleShareList)
	mcp.AddTool(s.mcpServer, newShareRevokeTool(), s.handleShareRevoke)
	mcp.AddTool(s.mcpServer, newInvoiceListTool(), s.handleInvoiceList)
	mcp.AddTool(s.mcpServer, newInvoiceTool(), s.handleInvoice)
	mcp.AddTool(s.mcpServer, newPricingTool(), s.handlePricing)

	// Register browser-backed visualization tools (local/stdio mode only).
	if s.browser != nil {
		mcp.AddTool(s.mcpServer, newVisualizeTool(), s.handleVisualize)
		mcp.AddTool(s.mcpServer, newUIStateTool(), s.handleUIState)
	}
}

// analyticsMiddleware tracks analytics for all MCP requests
func (s *Server) analyticsMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, runErr error) {
		// Reload config and client for this request
		cfg, client, projectID, err := s.app.Load(ctx)
		if err != nil {
			// If config loading fails, skip analytics and continue
			return next(ctx, method, req)
		}
		a := analytics.New(cfg, client, projectID)

		start := time.Now()
		switch r := req.(type) {
		case *mcp.CallToolRequest:
			// Extract arguments from the tool call
			var args map[string]any
			if len(r.Params.Arguments) > 0 {
				if err := json.Unmarshal(r.Params.Arguments, &args); err != nil {
					s.logger.Error("Error unmarshaling tool call arguments", slog.String("error", err.Error()))
				}
			}

			defer func() {
				toolErr := runErr
				if callToolResult, ok := result.(*mcp.CallToolResult); ok && callToolResult != nil && callToolResult.IsError && len(callToolResult.Content) > 0 {
					if textContent, ok := callToolResult.Content[0].(*mcp.TextContent); ok && textContent != nil {
						toolErr = errors.New(textContent.Text)
					}
				}

				a.Track(fmt.Sprintf("Call %s tool", r.Params.Name),
					analytics.Map(args),
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.Error(toolErr),
				)
			}()
		case *mcp.ReadResourceRequest:
			defer func() {
				a.Track("Read proxied resource",
					analytics.Property("resource_uri", r.Params.URI),
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.Error(runErr),
				)
			}()
		case *mcp.GetPromptRequest:
			defer func() {
				a.Track(fmt.Sprintf("Get %s prompt", r.Params.Name),
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.Error(runErr),
				)
			}()
		}

		// Execute the actual handler
		return next(ctx, method, req)
	}
}

// Close gracefully shuts down the MCP server and all proxy connections
func (s *Server) Close() error {
	var errs []error

	// Tear down the in-process web UI, if it was started.
	if s.browser != nil {
		if err := s.browser.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close web server: %w", err))
		}
	}

	// Close docs proxy connection
	if err := s.docsProxyClient.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close docs proxy client: %w", err))
	}

	return errors.Join(errs...)
}
