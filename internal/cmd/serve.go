package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve"
)

func buildServeCmd(app *common.App) *cobra.Command {
	var port int
	var host string
	var noOpen bool
	var logLevel slog.Level

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Launch a local web UI for running SQL queries",
		Long: `Start a local web server and open a browser to a UI that lets you run SQL
queries against your ghost databases. The server runs only for the duration
of this command — press Ctrl+C to stop it.`,
		Example: `  # Launch on an auto-picked port and open the browser
  ghost serve

  # Pin a port and skip the browser
  ghost serve --port 5174 --no-open`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, _, err := app.GetClient(); err != nil {
				return err
			}

			// serve is a long-running process (like `ghost mcp`), so structured
			// logging to stderr is appropriate. The logger is passed directly to
			// the server, store, and handler; the handler's logging middleware is
			// responsible for putting a request-scoped logger on the context.
			logger := log.NewWithLevel(cmd.ErrOrStderr(), logLevel)

			if host != "127.0.0.1" && host != "localhost" && host != "::1" {
				logger.Warn("Binding to a non-loopback address exposes the SQL UI to your network; consider using 127.0.0.1",
					slog.String("host", host),
				)
			}

			// configDir is where the serve UI state file is persisted.
			configDir := app.GetConfig().ConfigDir
			store := serve.NewStore(configDir, logger)
			defer store.Close() // Close any open database sessions/runs held by the store.

			handler := serve.NewHandler(serve.HandlerConfig{
				App:    app,
				Store:  store,
				Logger: logger,
			})
			server := serve.NewServer(host, port, handler.Handler(), logger)
			defer server.Close()

			if err := server.Start(cmd.Context()); err != nil {
				return err
			}

			url := server.URL()
			logger.Info("Listening", slog.String("url", url))

			if !noOpen {
				if err := common.OpenBrowser(url); err != nil {
					logger.Warn("Failed to open browser", slog.Any("error", err))
				}
			}
			logger.Info("Press Ctrl+C to stop")

			// Wait for a shutdown signal (context cancellation) or an error
			// serving HTTP requests.
			select {
			case err := <-server.Errors():
				if err != nil {
					return fmt.Errorf("error serving requests: %w", err)
				}
			case <-cmd.Context().Done():
			}

			// Immediately shut down the server, closing any active connections
			// without waiting for in-flight requests to complete.
			return server.Close()
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "TCP port to listen on (0 = auto)")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "interface to bind (loopback by default)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open the browser")
	cmd.Flags().TextVar(&logLevel, "log-level", slog.LevelInfo, "log level: debug, info, warn, or error")

	if err := cmd.RegisterFlagCompletionFunc("log-level", logLevelCompletion); err != nil {
		panic(err)
	}

	return cmd
}
