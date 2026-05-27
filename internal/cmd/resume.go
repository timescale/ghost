package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
)

func buildResumeCmd(app *common.App) *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:   "resume <name-or-id>",
		Short: "Resume a paused database",
		Long:  `Resume a paused database to accept connections again.`,
		Example: `  # Resume a database
  ghost resume my-database

  # Resume and wait for the database to be ready
  ghost resume my-database --wait`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: databaseCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			databaseRef := args[0]

			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			// Make the resume request
			resp, err := client.ResumeDatabaseWithResponse(
				cmd.Context(),
				api.SpaceId(projectID),
				api.DatabaseRef(databaseRef),
			)
			if err != nil {
				return fmt.Errorf("failed to resume database: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}

			if resp.JSON202 == nil {
				return errors.New("empty response from API")
			}
			database := *resp.JSON202

			cmd.Printf("Resuming '%s' (%s)...\n", database.Name, database.Id)

			// Get password for database
			password, err := common.GetPassword(database, "tsdbadmin")
			if err != nil {
				cmd.PrintErrf("Warning: failed to get password: %v\n", err)
			}
			connStr, err := common.BuildConnectionString(common.ConnectionStringArgs{
				Database: database,
				Role:     "tsdbadmin",
				Password: password,
			})
			if err != nil {
				cmd.PrintErrf("Warning: failed to build connection string: %v\n", err)
			} else {
				cmd.Printf("Connection: %s\n", connStr)
			}

			if !wait {
				return nil
			}

			return common.WaitForDatabaseWithProgress(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), common.WaitForDatabaseArgs{
				Client:      client,
				ProjectID:   projectID,
				DatabaseRef: database.Id,
			})
		},
	}

	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the database to be ready before returning")

	return cmd
}
