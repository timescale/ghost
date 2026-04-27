package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildCreateDedicatedCmd(app *common.App) *cobra.Command {
	var name string
	var size string
	var shareToken string
	var jsonOutput bool
	var yamlOutput bool
	var wait bool

	cmd := &cobra.Command{
		Use:   "dedicated",
		Short: "Create a dedicated database",
		Long: `Create a new dedicated database. Dedicated databases are always-on,
billed instances that are not subject to space compute or storage limits.
A payment method must be on file.`,
		Example: `  # Create a dedicated database (default size: 1x)
  ghost create dedicated

  # Create with a specific size
  ghost create dedicated --size 2x

  # Create with a custom name
  ghost create dedicated --name myapp --size 4x

  # Create a dedicated database from a share token
  ghost create dedicated --from-share <token>

  # Create and output as JSON
  ghost create dedicated --json

  # Create and wait for the database to be ready
  ghost create dedicated --size 2x --wait`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createDatabase(cmd, app, createDatabaseArgs{
				req: api.CreateDatabaseRequest{
					Name:       util.PtrIfNonZero(name),
					Type:       new(api.DatabaseTypeDedicated),
					Size:       new(api.DatabaseSize(size)),
					ShareToken: util.PtrIfNonZero(shareToken),
				},
				jsonOutput: jsonOutput,
				yamlOutput: yamlOutput,
				wait:       wait,
			})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Database name (auto-generated if not provided)")
	cmd.Flags().StringVar(&size, "size", "1x", "Database size (1x, 2x, 4x, 8x)")
	cmd.Flags().StringVar(&shareToken, "from-share", "", "Create the database from a share token")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the database to be ready before returning")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	if err := cmd.RegisterFlagCompletionFunc("size", sizeCompletion); err != nil {
		panic(err)
	}

	return cmd
}
