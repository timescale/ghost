package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildCreateCmd(app *common.App) *cobra.Command {
	var name string
	var shareToken string
	var jsonOutput bool
	var yamlOutput bool
	var wait bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new Postgres database",
		Example: `  # Create a database with auto-generated name
  ghost create

  # Create a database with a custom name
  ghost create --name myapp

  # Create a database from a share token
  ghost create --from-share <token> --name myapp

  # Create and output as JSON
  ghost create --json

  # Create and output as YAML
  ghost create --yaml

  # Create and wait for the database to be ready
  ghost create --wait`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return createDatabase(cmd, app, createDatabaseArgs{
				req: api.CreateDatabaseRequest{
					Name:       util.PtrIfNonZero(name),
					ShareToken: util.PtrIfNonZero(shareToken),
				},
				jsonOutput: jsonOutput,
				yamlOutput: yamlOutput,
				wait:       wait,
			})
		},
	}

	// Add flags
	cmd.Flags().StringVar(&name, "name", "", "Database name (auto-generated if not provided)")
	cmd.Flags().StringVar(&shareToken, "from-share", "", "Create the database from a share token")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the database to be ready before returning")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	// Add subcommands
	cmd.AddCommand(buildCreateDedicatedCmd(app))

	return cmd
}

type createDatabaseArgs struct {
	req        api.CreateDatabaseRequest
	jsonOutput bool
	yamlOutput bool
	wait       bool
}

// DatabaseCreateOutput represents the output format for the create command
type DatabaseCreateOutput struct {
	Name       string           `json:"name"`
	ID         string           `json:"id"`
	Size       string           `json:"size,omitempty"`
	Connection string           `json:"connection"`
	Type       api.DatabaseType `json:"-"`
}

func createDatabase(cmd *cobra.Command, app *common.App, args createDatabaseArgs) error {
	client, projectID, err := app.GetClient()
	if err != nil {
		return err
	}

	// Make API call to create database
	resp, err := client.CreateDatabaseWithResponse(cmd.Context(), projectID, args.req)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusAccepted {
		return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}

	if resp.JSON202 == nil {
		return errors.New("empty response from API")
	}
	database := *resp.JSON202

	// Save password to .pgpass file
	password := util.Deref(database.Password)
	if password == "" {
		cmd.PrintErrf("Warning: no initial password returned by API\n")
	} else {
		if err := common.SavePassword(database, password, "tsdbadmin"); err != nil {
			cmd.PrintErrf("Warning: failed to save password to .pgpass: %v\n", err)
		}
	}

	// Build connection string
	connStr, err := common.BuildConnectionString(common.ConnectionStringArgs{
		Database: database,
		Role:     "tsdbadmin",
		Password: password,
	})
	if err != nil {
		cmd.PrintErrf("Warning: failed to build connection string: %v\n", err)
	}

	// Convert to output format
	output := DatabaseCreateOutput{
		Name:       database.Name,
		ID:         database.Id,
		Size:       util.DerefStr(args.req.Size), // TODO: use API response size when available
		Connection: connStr,
		Type:       database.Type,
	}

	// Output in requested format
	switch {
	case args.jsonOutput:
		if err := util.SerializeToJSON(cmd.OutOrStdout(), output); err != nil {
			return err
		}
	case args.yamlOutput:
		if err := util.SerializeToYAML(cmd.OutOrStdout(), output); err != nil {
			return err
		}
	default:
		outputDatabaseCreate(cmd, output)
	}

	if !args.wait {
		return nil
	}

	return common.WaitForDatabaseWithProgress(cmd.Context(), cmd.ErrOrStderr(), common.WaitForDatabaseArgs{
		Client:      client,
		ProjectID:   projectID,
		DatabaseRef: database.Id,
	})
}

// outputDatabaseCreate outputs the created database in the default text format
func outputDatabaseCreate(cmd *cobra.Command, output DatabaseCreateOutput) {
	dbType := ""
	if output.Type == api.DatabaseTypeDedicated {
		dbType = " dedicated"
	}

	sizeInfo := ""
	if output.Size != "" {
		sizeInfo = fmt.Sprintf(" (size: %s)", output.Size)
	}

	cmd.Printf("Created%s database '%s'%s\n", dbType, output.Name, sizeInfo)
	cmd.Printf("ID: %s\n", output.ID)
	if output.Connection != "" {
		cmd.Printf("Connection: %s\n", output.Connection)
	}
}
