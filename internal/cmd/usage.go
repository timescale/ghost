package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildUsageCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:     "usage",
		Aliases: []string{"status"},
		Short:   "Show space usage",
		Example: `  # Show space usage
  ghost usage

  # Output as JSON
  ghost usage --json

  # Output as YAML
  ghost usage --yaml`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			status, err := common.FetchStatus(cmd.Context(), client, projectID)
			if err != nil {
				return err
			}

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), status)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), status)
			default:
				outputUsage(cmd, status)
				return nil
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputUsage(cmd *cobra.Command, status common.Status) {
	computeHours := float64(status.ComputeMinutes) / 60
	computeLimitHours := float64(status.ComputeLimitMinutes) / 60
	computePercent := float64(status.ComputeMinutes) / float64(status.ComputeLimitMinutes) * 100

	storageMibInt := int(status.StorageMib)
	storageStr := common.FormatStorageSize(&storageMibInt)
	storagePercent := float64(status.StorageMib) / float64(status.StorageLimitMib) * 100

	// Build status breakdown (only non-zero counts, in consistent order)
	type statusEntry struct {
		name  string
		count int
	}
	entries := []statusEntry{
		{"queued", status.Databases.Queued},
		{"configuring", status.Databases.Configuring},
		{"running", status.Databases.Running},
		{"pausing", status.Databases.Pausing},
		{"paused", status.Databases.Paused},
		{"resuming", status.Databases.Resuming},
		{"deleting", status.Databases.Deleting},
		{"deleted", status.Databases.Deleted},
		{"upgrading", status.Databases.Upgrading},
		{"unstable", status.Databases.Unstable},
		{"unknown", status.Databases.Unknown},
	}

	var total int
	var parts []string
	for _, e := range entries {
		total += e.count
		if e.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", e.count, e.name))
		}
	}

	cmd.Printf("Space: %s\n", status.SpaceID)
	cmd.Printf("Compute: %g/%g hours (%s)\n", computeHours, computeLimitHours, formatPercent(computePercent))
	cmd.Printf("Storage: %s/1TiB (%s)\n", storageStr, formatPercent(storagePercent))
	if len(parts) > 0 {
		cmd.Printf("Databases: %d (%s)\n", total, strings.Join(parts, ", "))
	} else {
		cmd.Printf("Databases: %d\n", total)
	}
	// Show cost only when at least one field is non-zero. Free-tier users
	// usually have zero cost, and "$0.00" adds noise.
	costToDate := util.Deref(status.CostToDate)
	estimatedTotalCost := util.Deref(status.EstimatedTotalCost)
	if costToDate > 0 || estimatedTotalCost > 0 {
		cmd.Printf("Cost: $%.2f so far this cycle ($%.2f estimated total)\n",
			costToDate, estimatedTotalCost)
	}
}

// formatPercent formats a percentage value, dropping the trailing ".0" for whole numbers.
func formatPercent(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	s = strings.TrimSuffix(s, ".0")
	return s + "%"
}
