package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

func buildShareRevokeCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:               "revoke <share-token>",
		Short:             "Revoke a database share",
		Long:              `Revoke a share so its URL can no longer be used to create new forks.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			share, err := findShareByToken(cmd.Context(), client, projectID, args[0])
			if err != nil {
				return err
			}

			resp, err := client.RevokeShareWithResponse(cmd.Context(), projectID, share.Id)
			if err != nil {
				return fmt.Errorf("failed to revoke share: %w", err)
			}
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}
			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}
			output := toShare(*resp.JSON200, cfg.ShareURL, time.Now())

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), output)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), output)
			default:
				cmd.Printf("Revoked share for '%s'\n", output.DatabaseName)
				return nil
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

// findShareByToken looks up the API share matching the given token. The
// revoke API requires the internal share ID, so we list shares and match on
// token client-side to keep the ID out of the user-facing surface.
func findShareByToken(ctx context.Context, client api.ClientWithResponsesInterface, projectID, token string) (api.DatabaseShare, error) {
	resp, err := client.ListSharesWithResponse(ctx, projectID)
	if err != nil {
		return api.DatabaseShare{}, fmt.Errorf("failed to list shares: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return api.DatabaseShare{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return api.DatabaseShare{}, errors.New("empty response from API")
	}
	for _, s := range *resp.JSON200 {
		if s.ShareToken == token {
			return s, nil
		}
	}
	return api.DatabaseShare{}, fmt.Errorf("share not found for the given token")
}
