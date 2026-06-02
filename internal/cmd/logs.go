package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildLogsCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool
	var tail int
	var until time.Time

	cmd := &cobra.Command{
		Use:   "logs <name-or-id>",
		Short: "View logs for a database",
		Long: `View logs for a database.

Fetches and displays logs from the specified database. By default, shows the
last 500 log entries. Log lines are displayed in chronological order with the
most recent entries at the bottom.`,
		Example: `  # View last 500 logs
  ghost logs my-database

  # View last 50 lines
  ghost logs my-database --tail 50

  # View logs before a specific time
  ghost logs my-database --until 2024-01-15T10:00:00Z

  # View logs as JSON
  ghost logs my-database --json`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: databaseCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			databaseRef := args[0]

			if tail < 1 {
				return fmt.Errorf("--tail must be at least 1, got %d", tail)
			}

			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			// Fetch logs with pagination
			logs, err := common.FetchLogs(cmd.Context(), common.FetchLogsArgs{
				Client:      client,
				ProjectID:   projectID,
				DatabaseRef: databaseRef,
				Tail:        tail,
				Until:       until,
			})
			if err != nil {
				return err
			}

			// Output in requested format
			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), logs)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), logs)
			default:
				for _, line := range logs {
					cmd.Println(colorizeLogLine(line))
				}
				return nil
			}
		},
	}

	cmd.Flags().IntVar(&tail, "tail", 500, "Number of log lines to show")
	cmd.Flags().TimeVar(&until, "until", time.Time{}, []string{time.RFC3339}, "Fetch logs before this timestamp (RFC3339 format, e.g. 2024-01-15T10:00:00Z)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

// logLevelRegex matches PostgreSQL log levels in log lines.
var logLevelRegex = regexp.MustCompile(`\b(DEBUG|INFO|NOTICE|WARNING|LOG|ERROR|FATAL|PANIC|DETAIL|HINT|QUERY|CONTEXT|LOCATION):`)

// Log level styles
var (
	logErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Red)
	logWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	logInfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Blue)
	logDebugStyle   = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	logQueryStyle   = lipgloss.NewStyle().Foreground(lipgloss.Green)
	logLogStyle     = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	logDetailStyle  = lipgloss.NewStyle().Foreground(lipgloss.White)
)

// colorizeLogLine applies color to the log level in a PostgreSQL log line.
func colorizeLogLine(line string) string {
	return logLevelRegex.ReplaceAllStringFunc(line, func(match string) string {
		logLevel := strings.TrimSuffix(match, ":")

		var style lipgloss.Style
		switch logLevel {
		case "ERROR", "FATAL", "PANIC":
			style = logErrorStyle
		case "WARNING":
			style = logWarningStyle
		case "INFO", "NOTICE":
			style = logInfoStyle
		case "DEBUG":
			style = logDebugStyle
		case "QUERY":
			style = logQueryStyle
		case "LOG":
			style = logLogStyle
		case "DETAIL", "HINT", "CONTEXT", "LOCATION":
			style = logDetailStyle
		default:
			return match
		}

		return style.Render(logLevel) + ":"
	})
}
