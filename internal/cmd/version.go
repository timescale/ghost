package cmd

import (
	"fmt"
	"io"
	"runtime"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/util"
)

type VersionOutput struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

func buildVersionCmd(_ *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool
	var bareOutput bool

	cmd := &cobra.Command{
		Use:               "version",
		Short:             "Show version information",
		Long:              `Display version, build time, and git commit information for the Ghost CLI`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {

			versionOutput := VersionOutput{
				Version:   config.Version,
				BuildTime: config.BuildTime,
				GitCommit: config.GitCommit,
				GoVersion: runtime.Version(),
				Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			}

			output := cmd.OutOrStdout()
			switch {
			case bareOutput:
				cmd.Println(config.Version)
			case jsonOutput:
				if err := util.SerializeToJSON(output, versionOutput); err != nil {
					return err
				}
			case yamlOutput:
				if err := util.SerializeToYAML(output, versionOutput); err != nil {
					return err
				}
			default:
				if err := outputVersion(output, versionOutput); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.Flags().BoolVar(&bareOutput, "bare", false, "Print only the version string")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml", "bare")

	return cmd
}

func outputVersion(w io.Writer, versionOutput VersionOutput) error {
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

	table.Append("Ghost CLI Version", versionOutput.Version)
	table.Append("Build Time", versionOutput.BuildTime)
	table.Append("Git Commit", versionOutput.GitCommit)
	table.Append("Go Version", versionOutput.GoVersion)
	table.Append("Platform", versionOutput.Platform)

	return table.Render()
}
