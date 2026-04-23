package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildShareListCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List database shares",
		Long:              `List all shares in your Ghost space.`,
		Aliases:           []string{"ls"},
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			resp, err := client.ListSharesWithResponse(cmd.Context(), projectID)
			if err != nil {
				return fmt.Errorf("failed to list shares: %w", err)
			}
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}
			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}

			shares := *resp.JSON200
			if len(shares) == 0 {
				cmd.Println("No shares found.")
				cmd.Println("Run 'ghost share <database>' to create a share.")
				return nil
			}

			now := time.Now()
			output := make([]Share, len(shares))
			for i, s := range shares {
				share, err := toShare(s, cfg.ShareURL, now)
				if err != nil {
					return err
				}
				output[i] = share
			}

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), output)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), output)
			default:
				return outputShares(cmd.OutOrStdout(), output)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputShares(w io.Writer, shares []Share) error {
	table := tablewriter.NewTable(w,
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithPadding(tw.Padding{Left: "", Right: "  ", Overwrite: true}),
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{
				Left:   tw.Off,
				Right:  tw.Off,
				Top:    tw.Off,
				Bottom: tw.Off,
			},
			Settings: tw.Settings{
				Separators: tw.Separators{
					ShowHeader:     tw.Off,
					ShowFooter:     tw.Off,
					BetweenRows:    tw.Off,
					BetweenColumns: tw.Off,
				},
				Lines: tw.Lines{
					ShowHeaderLine: tw.Off,
				},
			},
		}),
	)

	table.Header("DATABASE", "STATUS", "CREATED", "EXPIRES", "TOKEN")
	for _, s := range shares {
		expires := "never"
		if s.ExpiresAt != nil {
			expires = s.ExpiresAt.Format(time.RFC3339)
		}
		table.Append(s.DatabaseName, s.Status, s.CreatedAt.Format(time.RFC3339), expires, s.ShareToken)
	}
	return table.Render()
}
