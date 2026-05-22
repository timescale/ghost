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

			usage, err := common.FetchUsage(cmd.Context(), client, projectID)
			if err != nil {
				return err
			}

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), usage)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), usage)
			default:
				outputUsage(cmd, usage)
				return nil
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputUsage(cmd *cobra.Command, usage common.Usage) {
	computeHours := float64(usage.ComputeMinutes) / 60
	computeLimitHours := float64(usage.ComputeLimitMinutes) / 60
	computePercent := float64(usage.ComputeMinutes) / float64(usage.ComputeLimitMinutes) * 100

	storageMibInt := int(usage.StorageMib)
	storageStr := common.FormatStorageSize(&storageMibInt)
	storagePercent := float64(usage.StorageMib) / float64(usage.StorageLimitMib) * 100

	// Build status breakdown (only non-zero counts, in consistent order)
	type statusEntry struct {
		name  string
		count int
	}
	entries := []statusEntry{
		{"queued", usage.Databases.Queued},
		{"configuring", usage.Databases.Configuring},
		{"running", usage.Databases.Running},
		{"pausing", usage.Databases.Pausing},
		{"paused", usage.Databases.Paused},
		{"resuming", usage.Databases.Resuming},
		{"deleting", usage.Databases.Deleting},
		{"deleted", usage.Databases.Deleted},
		{"upgrading", usage.Databases.Upgrading},
		{"unstable", usage.Databases.Unstable},
		{"unknown", usage.Databases.Unknown},
	}

	var total int
	var parts []string
	for _, e := range entries {
		total += e.count
		if e.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", e.count, e.name))
		}
	}

	cmd.Printf("Space: %s\n", usage.SpaceID)
	cmd.Printf("Compute: %g/%g hours (%s)\n", computeHours, computeLimitHours, formatPercent(computePercent))
	cmd.Printf("Storage: %s/1TiB (%s)\n", storageStr, formatPercent(storagePercent))
	if len(parts) > 0 {
		cmd.Printf("Databases: %d (%s)\n", total, strings.Join(parts, ", "))
	} else {
		cmd.Printf("Databases: %d\n", total)
	}
	// Show cost only when at least one field is non-zero. Free-tier users
	// usually have zero cost, and "$0.00" adds noise.
	costToDate := util.Deref(usage.CostToDate)
	estimatedTotalCost := util.Deref(usage.EstimatedTotalCost)
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
