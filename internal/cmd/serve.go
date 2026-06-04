package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/serve"
)

func buildServeCmd(app *common.App) *cobra.Command {
	var port int
	var host string
	var noOpen bool

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
			if host != "127.0.0.1" && host != "localhost" && host != "::1" {
				cmd.PrintErrf("Warning: binding to %q exposes the SQL UI to your network. Consider using 127.0.0.1.\n", host)
			}

			srv, err := serve.New(serve.Config{
				Host:   host,
				Port:   port,
				App:    app,
				Logger: newLogger(cmd),
			})
			if err != nil {
				return err
			}

			url := srv.URL()
			cmd.PrintErrf("Listening on %s\n", url)

			if !noOpen {
				if err := common.OpenBrowser(url); err != nil {
					cmd.PrintErrf("Failed to open browser: %v\n", err)
				}
			}
			cmd.PrintErrln("Press Ctrl+C to stop.")

			return srv.Serve(cmd.Context())
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "TCP port to listen on (0 = auto)")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "interface to bind (loopback by default)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open the browser")

	return cmd
}
