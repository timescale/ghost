package cmd

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/util"
)

func buildConfigCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool
	var envFlag bool
	var allFlag bool

	cmd := &cobra.Command{
		Use:               "config",
		Short:             "List current configuration",
		Long:              `Display the current configuration settings`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.GetConfig()

			cfgOut, err := config.LoadForOutput(cfg.ConfigDir, envFlag, allFlag)
			if err != nil {
				return err
			}

			output := cmd.OutOrStdout()
			switch {
			case jsonOutput:
				return util.SerializeToJSON(output, cfgOut)
			case yamlOutput:
				return util.SerializeToYAML(output, cfgOut)
			default:
				return outputConfigTable(output, cfgOut)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")
	cmd.Flags().BoolVar(&envFlag, "env", false, "Apply environment variable overrides")
	cmd.Flags().BoolVar(&allFlag, "all", false, "Include internal config values")
	cmd.Flags().MarkHidden("all")

	cmd.AddCommand(buildConfigSetCmd(app))
	cmd.AddCommand(buildConfigUnsetCmd(app))
	cmd.AddCommand(buildConfigResetCmd(app))

	return cmd
}

func outputConfigTable(w io.Writer, cfg *config.ConfigOutput) error {
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

	if cfg.Analytics != nil {
		table.Append("analytics", fmt.Sprintf("%t", *cfg.Analytics))
	}
	if cfg.Color != nil {
		table.Append("color", fmt.Sprintf("%t", *cfg.Color))
	}
	if cfg.ReadOnly != nil {
		table.Append("read_only", fmt.Sprintf("%t", *cfg.ReadOnly))
	}
	if cfg.VersionCheck != nil {
		table.Append("version_check", fmt.Sprintf("%t", *cfg.VersionCheck))
	}
	if cfg.APIURL != nil {
		table.Append("api_url", *cfg.APIURL)
	}
	if cfg.DocsMCPURL != nil {
		table.Append("docs_mcp_url", *cfg.DocsMCPURL)
	}
	if cfg.ReleasesURL != nil {
		table.Append("releases_url", *cfg.ReleasesURL)
	}
	if cfg.ShareURL != nil {
		table.Append("share_url", *cfg.ShareURL)
	}
	return table.Render()
}
