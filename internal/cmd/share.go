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

// Share represents a database share in CLI output. The share_token (also
// embedded in the URL) is the only identifier exposed — it's what a recipient
// passes to 'ghost create', and it's also what you pass back to
// 'ghost share revoke' to revoke the share.
type Share struct {
	URL          string     `json:"url"`
	ShareToken   string     `json:"share_token"`
	DatabaseID   string     `json:"database_id"`
	DatabaseName string     `json:"database_name"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

func buildShareCmd(app *common.App) *cobra.Command {
	var expires string
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:   "share <name-or-id>",
		Short: "Share a database",
		Long: `Share a database so a recipient can create their own database from a snapshot.

The share URL can be handed to anyone — they don't need access to this space.
Whoever opens the URL gets instructions to run 'ghost create --from-share <token>'
(or 'ghost create dedicated --from-share <token>'), which spins up a new database
in their own space from the shared snapshot.`,
		Example: `  # Share a database (no expiry)
  ghost share my-database

  # Share for 24 hours (relative duration)
  ghost share my-database --expires 24h

  # Share until a specific time (RFC3339)
  ghost share my-database --expires 2026-05-01T00:00:00Z

  # Output as JSON
  ghost share my-database --json

  # The recipient creates their own database from the share token
  ghost create --from-share <token>`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: databaseCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			expiresAt, err := common.ParseExpires(expires, time.Now())
			if err != nil {
				return err
			}
			return runShareCreate(cmd, app, args[0], expiresAt, jsonOutput, yamlOutput)
		},
	}

	cmd.Flags().StringVar(&expires, "expires", "", "Expiry as a duration (e.g. 30m, 24h) or RFC3339 timestamp (e.g. 2026-05-01T00:00:00Z)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	cmd.AddCommand(buildShareListCmd(app))
	cmd.AddCommand(buildShareRevokeCmd(app))

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
	output, err := toShare(*resp.JSON201, cfg.ShareURL, time.Now())
	if err != nil {
		return err
	}

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
	cmd.Printf("Token: %s\n", o.ShareToken)
	if o.ExpiresAt != nil {
		cmd.Printf("Expires: %s\n", o.ExpiresAt.Format(time.RFC3339))
	}
	cmd.Println()
	cmd.Println("Send this URL to a human or agent to let them spin up their own copy of the database:")
	cmd.Println(o.URL)
}

func shareStatus(s api.DatabaseShare, now time.Time) string {
	if s.RevokedAt != nil {
		return "revoked"
	}
	if s.ExpiresAt != nil && now.After(*s.ExpiresAt) {
		return "expired"
	}
	return "active"
}

// toShare converts an API share into the CLI output shape, computing the
// status relative to now.
func toShare(s api.DatabaseShare, baseURL string, now time.Time) (Share, error) {
	u, err := common.ShareURL(baseURL, s.ShareToken, s.DatabaseName)
	if err != nil {
		return Share{}, err
	}
	return Share{
		URL:          u,
		ShareToken:   s.ShareToken,
		DatabaseID:   s.DatabaseId,
		DatabaseName: s.DatabaseName,
		Status:       shareStatus(s, now),
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
		RevokedAt:    s.RevokedAt,
	}, nil
}
