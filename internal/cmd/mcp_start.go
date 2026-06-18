package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/mcp"
)

// buildMCPStartCmd creates the start subcommand with transport options
func buildMCPStartCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Ghost MCP server",
		Long:  `Start the Ghost MCP server. Uses stdio transport by default.`,
		Example: `  # Start with stdio transport (default)
  ghost mcp start

  # Start with stdio transport (explicit)
  ghost mcp start stdio

  # Start with HTTP transport
  ghost mcp start http`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior when no subcommand is specified - use stdio
			return startStdioServer(cmd, app)
		},
	}

	// Add transport subcommands
	cmd.AddCommand(buildMCPStdioCmd(app))
	cmd.AddCommand(buildMCPHTTPCmd(app))

	return cmd
}

// buildMCPStdioCmd creates the stdio subcommand
func buildMCPStdioCmd(app *common.App) *cobra.Command {
	return &cobra.Command{
		Use:   "stdio",
		Short: "Start MCP server with stdio transport",
		Long:  `Start the MCP server using standard input/output transport.`,
		Example: `  # Start with stdio transport
  ghost mcp start stdio`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return startStdioServer(cmd, app)
		},
	}
}

// buildMCPHTTPCmd creates the http subcommand with port/host flags
func buildMCPHTTPCmd(app *common.App) *cobra.Command {
	var httpPort int
	var httpHost string

	cmd := &cobra.Command{
		Use:   "http",
		Short: "Start MCP server with HTTP transport",
		Long:  `Start the MCP server using the Streamable HTTP transport.`,
		Example: `  # Start HTTP server on default port 8080
  ghost mcp start http

  # Start HTTP server on custom port
  ghost mcp start http --port 3001

  # Start HTTP server on all interfaces
  ghost mcp start http --host 0.0.0.0 --port 8080

  # Start server and bind to specific interface
  ghost mcp start http --host 192.168.1.100 --port 9000`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		SilenceErrors:     true, // HTTP server uses slog for all output, including errors
		RunE: func(cmd *cobra.Command, args []string) error {
			return startHTTPServer(cmd, app, httpHost, httpPort)
		},
	}

	// Add HTTP-specific flags
	cmd.Flags().IntVar(&httpPort, "port", 8080, "Port to run HTTP server on")
	cmd.Flags().StringVar(&httpHost, "host", "localhost", "Host to bind to")

	return cmd
}

// startStdioServer starts the MCP server with stdio transport
func startStdioServer(cmd *cobra.Command, app *common.App) error {
	ctx := cmd.Context()
	// Create MCP server. Local (stdio) mode enables the browser-backed
	// visualization tools, since we can open a browser on the user's machine.
	server, err := mcp.NewServerWithOptions(ctx, app, log.New(cmd.ErrOrStderr()), mcp.Options{Local: true})
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}
	defer server.Close()

	// Start the stdio transport
	if err := server.StartStdio(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	// Close the MCP server when finished
	if err := server.Close(); err != nil {
		return fmt.Errorf("failed to close MCP server: %w", err)
	}
	return nil
}

// startHTTPServer starts the MCP server with HTTP transport
func startHTTPServer(cmd *cobra.Command, app *common.App, host string, port int) error {
	ctx := cmd.Context()
	logger := log.New(cmd.ErrOrStderr())

	// Create MCP server
	server, err := mcp.NewServer(ctx, app, logger)
	if err != nil {
		logger.Error("failed to create MCP server", slog.String("error", err.Error()))
		return fmt.Errorf("failed to create MCP server: %w", err)
	}
	defer server.Close()

	address := fmt.Sprintf("%s:%d", host, port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		logger.Error("failed to listen on port",
			slog.String("address", address),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to listen on port: %w", err)
	}
	defer listener.Close()

	// Create HTTP server
	httpServer := &http.Server{
		Handler: server.HTTPHandler(),
	}

	logger.Info("Ghost MCP server started", slog.String("address", address))
	logger.Info("Use Ctrl+C to stop the server")

	// Start server in goroutine using the existing listener
	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for context cancellation or a server error
	select {
	case err := <-errCh:
		logger.Error("HTTP server error", slog.String("error", err.Error()))
		return fmt.Errorf("HTTP server error: %w", err)
	case <-ctx.Done():
	}

	// Shutdown server gracefully
	logger.Info("Shutting down HTTP server")
	if err := httpServer.Shutdown(context.Background()); err != nil {
		logger.Error("failed to shut down HTTP server", slog.String("error", err.Error()))
		return fmt.Errorf("failed to shut down HTTP server: %w", err)
	}

	// Close the MCP server when finished
	if err := server.Close(); err != nil {
		logger.Error("failed to close MCP server", slog.String("error", err.Error()))
		return fmt.Errorf("failed to close MCP server: %w", err)
	}
	return nil
}
