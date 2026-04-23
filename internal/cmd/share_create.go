package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildShareCreateCmd(app *common.App) *cobra.Command {
	var expiresIn time.Duration
	var expiresAtStr string
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:   "share <name-or-id>",
		Short: "Share a database",
		Long: `Share a database so a recipient can create their own fork from a snapshot.

The share URL can be handed to anyone — they don't need access to this space.
Whoever opens the URL gets instructions to run 'ghost create --share-token <token>'
(or 'ghost create dedicated --share-token <token>'), which spins up a new database
in their own space from the shared snapshot.`,
		Example: `  # Share a database (no expiry)
  ghost share my-database

  # Share for 24 hours
  ghost share my-database --expires-in 24h

  # Share until a specific time
  ghost share my-database --expires-at 2026-05-01T00:00:00Z

  # Output as JSON
  ghost share my-database --json

  # The recipient creates their own database from the share token
  ghost create --share-token <token>`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: databaseCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var expiresAt *time.Time
			switch {
			case expiresIn > 0:
				t := time.Now().Add(expiresIn).UTC()
				expiresAt = &t
			case expiresAtStr != "":
				t, err := time.Parse(time.RFC3339, expiresAtStr)
				if err != nil {
					return fmt.Errorf("invalid --expires-at (expected RFC3339 like 2026-05-01T00:00:00Z): %w", err)
				}
				expiresAt = &t
			}

			return runShareCreate(cmd, app, args[0], expiresAt, jsonOutput, yamlOutput)
		},
	}

	cmd.Flags().DurationVar(&expiresIn, "expires-in", 0, "Relative expiry (e.g. 30m, 24h)")
	cmd.Flags().StringVar(&expiresAtStr, "expires-at", "", "Absolute expiry as RFC3339 (e.g. 2026-05-01T00:00:00Z)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")
	cmd.MarkFlagsMutuallyExclusive("expires-in", "expires-at")

	return cmd
}

func runShareCreate(cmd *cobra.Command, app *common.App, databaseRef string, expiresAt *time.Time, jsonOutput, yamlOutput bool) error {
	cfg, client, projectID, err := app.GetAll()
	if err != nil {
		return err
	}

	// Fetch source database to check readiness (sharing snapshots the DB)
	getResp, err := client.GetDatabaseWithResponse(cmd.Context(), projectID, databaseRef)
	if err != nil {
		return fmt.Errorf("failed to get database details: %w", err)
	}
	if getResp.StatusCode() != http.StatusOK {
		return common.ExitWithErrorFromStatusCode(getResp.StatusCode(), getResp.JSONDefault)
	}
	if getResp.JSON200 == nil {
		return errors.New("empty response from API")
	}
	database := *getResp.JSON200

	if err := common.CheckReady(database); err != nil {
		return handleDatabaseError(err, database.Id)
	}

	resp, err := client.ShareDatabaseWithResponse(cmd.Context(), projectID, database.Id, api.ShareDatabaseJSONRequestBody{
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return fmt.Errorf("failed to create share: %w", err)
	}
	if resp.StatusCode() != http.StatusCreated {
		return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON201 == nil {
		return errors.New("empty response from API")
	}
	output := toShare(*resp.JSON201, cfg.ShareURL, time.Now())

	switch {
	case jsonOutput:
		return util.SerializeToJSON(cmd.OutOrStdout(), output)
	case yamlOutput:
		return util.SerializeToYAML(cmd.OutOrStdout(), output)
	default:
		outputShareText(cmd, output)
		return nil
	}
}

func outputShareText(cmd *cobra.Command, o Share) {
	cmd.Printf("Shared '%s'\n", o.DatabaseName)
	cmd.Printf("URL: %s\n", o.URL)
	cmd.Printf("Token: %s\n", o.ShareToken)
	if o.ExpiresAt != nil {
		cmd.Printf("Expires: %s\n", o.ExpiresAt.Format(time.RFC3339))
	} else {
		cmd.Printf("Expires: never\n")
	}
}
