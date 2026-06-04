package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// DatabaseListItem represents a database in the list command output
type DatabaseListItem struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Type           api.DatabaseType   `json:"type"`
	Size           *api.DatabaseSize  `json:"size,omitempty"`
	Status         api.DatabaseStatus `json:"status"`
	StorageMib     *int               `json:"storage_mib"`
	ComputeMinutes *int64             `json:"compute_minutes,omitempty"`
}

func buildListCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all databases",
		Long:    `List all databases, including each database's current status, storage usage, and compute hours used in the current billing cycle.`,
		Example: `  # List all databases
  ghost list

  # List as JSON
  ghost list --json

  # List as YAML
  ghost list --yaml`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			// Make API call to list databases
			resp, err := client.ListDatabasesWithResponse(cmd.Context(), projectID)
			if err != nil {
				return fmt.Errorf("failed to list databases: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}

			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}
			databases := *resp.JSON200

			output := make([]DatabaseListItem, len(databases))
			for i, database := range databases {
				output[i] = DatabaseListItem{
					ID:             database.Id,
					Name:           database.Name,
					Type:           database.Type,
					Size:           database.Size,
					Status:         database.Status,
					StorageMib:     database.StorageMib,
					ComputeMinutes: database.ComputeMinutes,
				}
			}

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), output)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), output)
			default:
				return outputDatabaseList(cmd.OutOrStdout(), output)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputDatabaseList(w io.Writer, databases []DatabaseListItem) error {
	var standard, dedicated []DatabaseListItem
	for _, db := range databases {
		if db.Type == api.DatabaseTypeDedicated {
			dedicated = append(dedicated, db)
		} else {
			standard = append(standard, db)
		}
	}

	standardTable := common.NewTable(w)
	standardTable.Header("ID", "NAME", "STATUS", "STORAGE", "COMPUTE")
	for _, db := range standard {
		standardTable.Append(db.ID, db.Name, db.Status, common.FormatStorageSize(db.StorageMib), formatComputeHours(db.ComputeMinutes))
	}
	if err := standardTable.Render(); err != nil {
		return err
	}

	if len(dedicated) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, lipgloss.NewStyle().Bold(true).Render("Dedicated Databases"))
		dedicatedTable := common.NewTable(w)
		dedicatedTable.Header("ID", "NAME", "SIZE", "STATUS", "STORAGE")
		for _, db := range dedicated {
			sizeStr := "-"
			if db.Size != nil {
				sizeStr = string(*db.Size)
			}
			dedicatedTable.Append(db.ID, db.Name, sizeStr, db.Status, common.FormatStorageSize(db.StorageMib))
		}
		if err := dedicatedTable.Render(); err != nil {
			return err
		}
	}

	return nil
}

func formatComputeHours(minutes *int64) string {
	if minutes == nil {
		return "-"
	}
	return fmt.Sprintf("%gh", float64(*minutes)/60)
}
