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

func buildIDCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:     "id",
		Aliases: []string{"identity", "whoami", "who"},
		Short:   "Show the authenticated user or API key",
		Long: `Show information about the authenticated caller.

The output depends on how you are authenticated: logging in as a user shows your
user details, while authenticating with an API key shows details about the key
itself, such as its scope and the user who created it.`,
		Example: `  # Show the authenticated identity
  ghost id

  # Output as JSON
  ghost id --json

  # Output as YAML
  ghost id --yaml`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := app.GetClient()
			if err != nil {
				return err
			}

			resp, err := client.AuthInfoWithResponse(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get auth info: %w", err)
			}
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}
			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}
			info := resp.JSON200

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), info)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), info)
			default:
				outputID(cmd, info)
				return nil
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputID(cmd *cobra.Command, info *api.AuthInfo) {
	switch {
	case info.User != nil:
		cmd.Printf("Type: OAuth\n")
		cmd.Printf("User: %s (%s)\n", info.User.Name, info.User.Email)
	case info.ApiKey != nil:
		cmd.Printf("Type: API Key\n")
		cmd.Printf("Name: %s\n", info.ApiKey.Name)
		cmd.Printf("Prefix: %s\n", info.ApiKey.Prefix)
		cmd.Printf("Space: %s (%s)\n", info.ApiKey.SpaceName, info.ApiKey.SpaceId)
		cmd.Printf("User: %s (%s)\n", info.ApiKey.UserName, info.ApiKey.UserEmail)
		cmd.Printf("Created: %s\n", info.ApiKey.CreatedAt)
	default:
		// No user or API key details (shouldn't happen for a valid response),
		// but report the type so the output isn't empty.
		cmd.Printf("Type: %s\n", info.Type)
	}
}
