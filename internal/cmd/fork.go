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

func buildForkCmd(app *common.App) *cobra.Command {
	var name string
	var jsonOutput bool
	var yamlOutput bool
	var wait bool

	cmd := &cobra.Command{
		Use:   "fork <name-or-id> [new-name]",
		Short: "Fork a database",
		Long: `Fork an existing database to create a new independent copy.

To fork into an always-on dedicated database (not subject to space compute or
storage limits), use 'ghost fork-dedicated' instead.`,
		Example: `  # Fork a database with auto-generated name
  ghost fork my-database

  # Fork a database with a custom name
  ghost fork my-database myapp-experiment

  # Fork and output as JSON
  ghost fork my-database --json

  # Fork and output as YAML
  ghost fork my-database --yaml

  # Fork and wait for the database to be ready
  ghost fork my-database --wait`,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: databaseCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := resolveDatabaseName(args, 1, name)
			if err != nil {
				return err
			}
			return forkDatabase(cmd, app, forkDatabaseArgs{
				sourceDatabaseRef: args[0],
				req: api.ForkDatabaseRequest{
					Name: util.PtrIfNonZero(name),
				},
				jsonOutput: jsonOutput,
				yamlOutput: yamlOutput,
				wait:       wait,
			})
		},
	}

	// Add flags
	cmd.Flags().StringVar(&name, "name", "", "Name for the forked database (auto-generated if not provided)")
	if err := cmd.Flags().MarkHidden("name"); err != nil {
		panic(err)
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the database to be ready before returning")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

type forkDatabaseArgs struct {
	sourceDatabaseRef string
	req               api.ForkDatabaseRequest
	jsonOutput        bool
	yamlOutput        bool
	wait              bool
}

// DatabaseForkOutput represents the output format for the fork command
type DatabaseForkOutput struct {
	SourceName string           `json:"source_name"`
	Name       string           `json:"name"`
	ID         string           `json:"id"`
	Size       string           `json:"size,omitempty"`
	Connection string           `json:"connection"`
	Type       api.DatabaseType `json:"-"`
}

func forkDatabase(cmd *cobra.Command, app *common.App, args forkDatabaseArgs) error {
	client, projectID, err := app.GetClient()
	if err != nil {
		return err
	}

	// Fetch source database to get its name and check readiness
	sourceResp, err := client.GetDatabaseWithResponse(cmd.Context(), projectID, args.sourceDatabaseRef)
	if err != nil {
		return fmt.Errorf("failed to get source database: %w", err)
	}
	if sourceResp.StatusCode() != http.StatusOK {
		return common.ExitWithErrorFromStatusCode(sourceResp.StatusCode(), sourceResp.JSONDefault)
	}
	if sourceResp.JSON200 == nil {
		return errors.New("empty response from API")
	}
	sourceDatabase := *sourceResp.JSON200

	// Check if the source database is ready
	if err := common.CheckReady(sourceDatabase); err != nil {
		return handleDatabaseError(err, sourceDatabase.Id)
	}

	// Make API call to fork database
	forkResp, err := client.ForkDatabaseWithResponse(cmd.Context(), projectID, sourceDatabase.Id, args.req)
	if err != nil {
		return fmt.Errorf("failed to fork database: %w", err)
	}

	// Handle API response
	if forkResp.StatusCode() != http.StatusAccepted {
		if common.IsNoPaymentMethod(forkResp.JSONDefault) && util.Deref(args.req.Type) == api.DatabaseTypeDedicated {
			return common.NoPaymentMethodError("fork a dedicated database")
		}
		if common.IsComputeLimitExceeded(forkResp.JSONDefault) {
			return common.ComputeLimitExceededError("fork a database")
		}
		return common.ExitWithErrorFromStatusCode(forkResp.StatusCode(), forkResp.JSONDefault)
	}

	if forkResp.JSON202 == nil {
		return errors.New("empty response from API")
	}
	forkedDatabase := *forkResp.JSON202

	// Save password to .pgpass file
	password := util.Deref(forkedDatabase.Password)
	if password == "" {
		cmd.PrintErrf("Warning: no initial password returned by API\n")
	} else {
		if err := common.SavePassword(forkedDatabase, password, "tsdbadmin"); err != nil {
			cmd.PrintErrf("Warning: failed to save password to .pgpass: %v\n", err)
		}
	}

	// Build connection string
	connStr, err := common.BuildConnectionString(common.ConnectionStringArgs{
		Database: forkedDatabase,
		Role:     "tsdbadmin",
		Password: password,
	})
	if err != nil {
		cmd.PrintErrf("Warning: failed to build connection string: %v\n", err)
	}

	// Convert to output format
	output := DatabaseForkOutput{
		SourceName: sourceDatabase.Name,
		Name:       forkedDatabase.Name,
		ID:         forkedDatabase.Id,
		Size:       util.DerefStr(args.req.Size), // TODO: use API response size when available
		Connection: connStr,
		Type:       forkedDatabase.Type,
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
		outputDatabaseFork(cmd, output)
	}

	if !args.wait {
		return nil
	}

	return common.WaitForDatabaseWithProgress(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), common.WaitForDatabaseArgs{
		Client:      client,
		ProjectID:   projectID,
		DatabaseRef: forkedDatabase.Id,
	})
}

// outputDatabaseFork outputs the forked database in the default text format
func outputDatabaseFork(cmd *cobra.Command, output DatabaseForkOutput) {
	dbType := ""
	if output.Type == api.DatabaseTypeDedicated {
		dbType = " dedicated"
	}

	sizeInfo := ""
	if output.Size != "" {
		sizeInfo = fmt.Sprintf(" (size: %s)", output.Size)
	}

	cmd.Printf("Forked '%s' →%s '%s'%s\n", output.SourceName, dbType, output.Name, sizeInfo)
	cmd.Printf("ID: %s\n", output.ID)
	if output.Connection != "" {
		cmd.Printf("Connection: %s\n", output.Connection)
	}
}
