package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
)

func buildSchemaCmd(app *common.App) *cobra.Command {
	var (
		schemaName      string
		includeInternal bool
	)
	cmd := &cobra.Command{
		Use:   "schema <name-or-id>",
		Short: "Display database schema information",
		Long: `Display database schema information including tables, views, materialized views,
enum types, functions, and procedures with their columns, constraints, indexes,
and triggers. By default all user-visible schemas are shown; system schemas
(information_schema, pg_*, _timescaledb_*) and extension-owned objects are
excluded.`,
		Example: `  ghost schema my-database
  ghost schema my-database --schema reporting
  ghost schema my-database --internal`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: databaseCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			databaseRef := args[0]

			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			schema, err := common.FetchDatabaseSchema(cmd.Context(), common.FetchDatabaseSchemaArgs{
				Client:          client,
				ProjectID:       projectID,
				DatabaseRef:     databaseRef,
				Schema:          schemaName,
				IncludeInternal: includeInternal,
			})
			if err != nil {
				return handleDatabaseError(err, databaseRef)
			}

			cmd.Print(common.FormatSchema(schema))
			return nil
		},
	}

	cmd.Flags().StringVar(&schemaName, "schema", "", "Restrict output to a single Postgres schema")
	cmd.Flags().BoolVar(&includeInternal, "internal", false, "Include system schemas (information_schema, pg_*, _timescaledb_*) and extension-owned objects")

	return cmd
}
