package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
)

func buildSchemaCmd(app *common.App) *cobra.Command {
	var (
		schemaName         string
		includeInternal    bool
		includeDefinitions bool
		includeComments    bool
	)
	cmd := &cobra.Command{
		Use:   "schema <name-or-id>",
		Short: "Display database schema information",
		Long: `Display database schema information including tables (regular, partitioned, and
foreign), views, materialized views, enum types, functions, and procedures with
their columns, constraints, indexes, and triggers. Only objects the connecting user can access are listed. By default
system schemas (information_schema, pg_*, _timescaledb_*) and extension-owned
objects are excluded; use --schema to target a specific schema (including a
system schema such as pg_catalog) or --internal to include everything.

Object definitions (view SELECT statements and function/procedure bodies) are
omitted by default to keep the output concise; pass --definitions to include
them. Object comments (COMMENT ON text for schemas, tables, views, columns,
enums, functions, and procedures) are likewise omitted by default; pass
--comments to include them.`,
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
				Client:             client,
				ProjectID:          projectID,
				DatabaseRef:        databaseRef,
				Schema:             schemaName,
				IncludeInternal:    includeInternal,
				IncludeDefinitions: includeDefinitions,
				IncludeComments:    includeComments,
			})
			if err != nil {
				return handleDatabaseError(err, databaseRef)
			}

			cmd.Print(common.FormatSchema(schema))
			return nil
		},
	}

	cmd.Flags().StringVar(&schemaName, "schema", "", "Restrict output to a single Postgres schema (may be a system schema; only objects you can access are shown)")
	cmd.Flags().BoolVar(&includeInternal, "internal", false, "Include system schemas (information_schema, pg_*, _timescaledb_*) and extension-owned objects")
	cmd.Flags().BoolVar(&includeDefinitions, "definitions", false, "Include full object definitions (view SELECT statements and function/procedure bodies)")
	cmd.Flags().BoolVar(&includeComments, "comments", false, "Include object comments (COMMENT ON text for schemas, tables, views, columns, enums, functions, and procedures)")

	return cmd
}
