package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	"github.com/timescale/ghost/internal/analytics"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/util"
)

func Execute(ctx context.Context) error {
	rootCmd, _, err := buildRootCmd()
	if err != nil {
		return err
	}

	return rootCmd.ExecuteContext(ctx)
}

// BuildRootCmd exposes the root command for tools like doc generators.
func BuildRootCmd() (*cobra.Command, error) {
	cmd, _, err := buildRootCmd()
	return cmd, err
}

func buildRootCmd() (*cobra.Command, *common.App, error) {
	// Match command names and aliases case-insensitively (e.g. `ghost LIST`
	// works the same as `ghost list`). Cobra only exposes this as a global.
	cobra.EnableCaseInsensitive = true

	experimental, _ := strconv.ParseBool(os.Getenv("GHOST_EXPERIMENTAL"))

	app := &common.App{
		Experimental: experimental,
	}

	var configDirFlag string
	var analyticsFlag bool
	var colorFlag bool
	var versionCheckFlag bool

	stdoutWriter := util.NewTermWriter(os.Stdout)
	stderrWriter := util.NewTermWriter(os.Stderr)

	cmd := &cobra.Command{
		Use:   "ghost",
		Short: "CLI for managing Postgres databases",
		Long:  `Ghost is a command-line interface for managing PostgreSQL databases.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			app.SetFlags(cmd.Flags())

			// Load config and attempt API client creation (best-effort)
			cfg, _, _, err := app.Load(cmd.Context())
			if err != nil {
				return err
			}

			// Disable colors by setting the color profile to ASCII, which
			// causes the colorprofile writers (set below as stdout/stderr)
			// to strip ANSI color sequences from all output.
			if !cfg.Color {
				stdoutWriter.ColorProfile.Profile = colorprofile.Ascii
				stderrWriter.ColorProfile.Profile = colorprofile.Ascii
			}

			return nil
		},
	}

	// Wrap stdout and stderr in colorprofile writers that automatically
	// downsample or strip ANSI color sequences based on terminal capabilities.
	// All output via cmd.Print*/cmd.OutOrStdout() and cmd.PrintErr*/cmd.ErrOrStderr()
	// flows through these writers, giving us a single place to control color output.
	cmd.SetOut(stdoutWriter)
	cmd.SetErr(stderrWriter)

	// Match flag names case-insensitively (e.g. `--JSON` works the same as
	// `--json`). Propagates to all subcommands added after this point.
	cmd.SetGlobalNormalizationFunc(func(_ *flag.FlagSet, name string) flag.NormalizedName {
		return flag.NormalizedName(strings.ToLower(name))
	})

	// Add persistent flags
	cmd.PersistentFlags().StringVar(&configDirFlag, "config-dir", config.DefaultConfigDir, "config directory")
	cmd.PersistentFlags().BoolVar(&analyticsFlag, "analytics", true, "enable/disable usage analytics")
	cmd.PersistentFlags().BoolVar(&colorFlag, "color", true, "enable colored output")
	cmd.PersistentFlags().BoolVar(&versionCheckFlag, "version-check", true, "check for updates")

	// Add all subcommands
	cmd.AddCommand(buildVersionCmd(app))
	cmd.AddCommand(buildConfigCmd(app))
	cmd.AddCommand(buildMCPCmd(app))
	cmd.AddCommand(buildLoginCmd(app))
	cmd.AddCommand(buildLogoutCmd(app))
	cmd.AddCommand(buildCreateCmd(app))
	cmd.AddCommand(buildForkCmd(app))
	cmd.AddCommand(buildListCmd(app))
	cmd.AddCommand(buildStatusCmd(app))
	cmd.AddCommand(buildDeleteCmd(app))
	cmd.AddCommand(buildPauseCmd(app))
	cmd.AddCommand(buildResumeCmd(app))
	cmd.AddCommand(buildConnectCmd(app))
	cmd.AddCommand(buildPsqlCmd(app))
	cmd.AddCommand(buildSQLCmd(app))
	cmd.AddCommand(buildSchemaCmd(app))
	cmd.AddCommand(buildPasswordCmd(app))
	cmd.AddCommand(buildLogsCmd(app))
	cmd.AddCommand(buildFeedbackCmd(app))
	cmd.AddCommand(buildRenameCmd(app))
	cmd.AddCommand(buildShareCmd(app))
	cmd.AddCommand(buildApiKeyCmd(app))
	cmd.AddCommand(buildPaymentInteractiveCmd(app))
	cmd.AddCommand(buildUpgradeCmd(app))
	if app.Experimental {
		cmd.AddCommand(buildInvoiceCmd(app))
	}

	wrapCommands(cmd, app)

	return cmd, app, nil
}

func wrapCommands(cmd *cobra.Command, app *common.App) {
	// Wrap this command's RunE if it exists
	if cmd.RunE != nil {
		originalRunE := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) (runErr error) {
			// Perform version check
			defer versionCheck(c, app)()

			// Track analytics
			start := time.Now()
			defer func() {
				cfg, client, projectID := app.TryGetAll()
				a := analytics.New(cfg, client, projectID)
				a.Track(fmt.Sprintf("Run %s", c.CommandPath()),
					analytics.Args(c.CommandPath(), args),
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.FlagSet(c.Flags()),
					analytics.Error(runErr),
				)
			}()

			return originalRunE(c, args)
		}
	}

	// Recursively wrap all children
	for _, child := range cmd.Commands() {
		wrapCommands(child, app)
	}
}

func versionCheck(cmd *cobra.Command, app *common.App) func() {
	cfg := app.GetConfig()
	if !cfg.VersionCheck || !util.IsTerminal(cmd.ErrOrStderr()) {
		return func() {}
	}

	type result struct {
		result *common.VersionCheckResult
		err    error
	}
	versionCh := make(chan result, 1)

	go func() {
		res, err := common.CheckVersion(cmd.Context(), cfg.ReleasesURL)
		versionCh <- result{
			result: res,
			err:    err,
		}
	}()

	return func() {
		// Output version check result
		if vr, ok := <-versionCh; ok {
			if vr.err != nil && !errors.Is(vr.err, context.Canceled) {
				cmd.PrintErrf("Warning: failed to check for updates: %v\n", vr.err)
			} else if msg := vr.result.String(); msg != "" && cfg.VersionCheck {
				cmd.PrintErrln(msg)
			}
		}
	}
}
