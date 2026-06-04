package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// ApiKeyOutput represents the output format for a single API key.
type ApiKeyOutput struct {
	Prefix    string `json:"prefix"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func buildApiKeyListCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List API keys",
		Long:              `List all API keys for your Ghost space.`,
		Aliases:           []string{"ls"},
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, projectID, err := app.GetClient()
			if err != nil {
				return err
			}

			resp, err := client.ListApiKeysWithResponse(cmd.Context(), projectID)
			if err != nil {
				return fmt.Errorf("failed to list API keys: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
			}

			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}

			keys := *resp.JSON200
			if len(keys) == 0 {
				cmd.Println("No API keys found.")
				cmd.Println("Run 'ghost api-key create' to create an API key.")
				return nil
			}

			output := make([]ApiKeyOutput, len(keys))
			for i, key := range keys {
				output[i] = ApiKeyOutput{
					Prefix:    key.Prefix,
					Name:      key.Name,
					CreatedAt: key.CreatedAt.String(),
				}
			}

			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), output)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), output)
			default:
				return outputApiKeys(cmd.OutOrStdout(), output)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

func outputApiKeys(w io.Writer, keys []ApiKeyOutput) error {
	table := common.NewTable(w)

	table.Header("PREFIX", "NAME", "CREATED AT")
	for _, key := range keys {
		table.Append(key.Prefix, key.Name, key.CreatedAt)
	}

	return table.Render()
}
