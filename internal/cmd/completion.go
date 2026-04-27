package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/mcp"
)

func databaseCompletion(app *common.App) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Database name/ID is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		databases, err := listDatabases(cmd, app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Suggest each database at most once: prefer name, fall back to ID
		results := make([]string, 0, len(databases))
		for _, database := range databases {
			if strings.HasPrefix(database.Name, toComplete) {
				results = append(results, cobra.CompletionWithDesc(database.Name, database.Id))
			} else if strings.HasPrefix(database.Id, toComplete) {
				results = append(results, cobra.CompletionWithDesc(database.Id, database.Name))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	}
}

func listDatabases(cmd *cobra.Command, app *common.App) ([]api.Database, error) {
	client, projectID, err := app.GetClient()
	if err != nil {
		return nil, err
	}

	// Make API call to list databases
	resp, err := client.ListDatabasesWithResponse(cmd.Context(), projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}

	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	return *resp.JSON200, nil
}

func configOptionCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Config option is always first positional argument
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return filterCompletionsByPrefix(config.PublicConfigOptions(), toComplete), cobra.ShellCompDirectiveNoFileComp
}

func mcpCapabilityCompletion(app *common.App) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Capability name is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Create MCP server to get capabilities
		server, err := mcp.NewServer(cmd.Context(), app, nil)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer server.Close()

		capabilities, err := server.ListCapabilities(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Close the MCP server when finished
		if err := server.Close(); err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return filterCompletionsByPrefix(capabilities.Names(), toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func paymentMethodIDCompletion(app *common.App) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Payment method ID is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		methods, err := listPaymentMethods(cmd, app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		results := make([]string, 0, len(methods))
		for _, pm := range methods {
			if strings.HasPrefix(pm.Id, toComplete) {
				desc := fmt.Sprintf("%s ending in %s", pm.Brand, pm.Last4)
				results = append(results, cobra.CompletionWithDesc(pm.Id, desc))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	}
}

func listPaymentMethods(cmd *cobra.Command, app *common.App) ([]api.PaymentMethod, error) {
	client, projectID, err := app.GetClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.ListPaymentMethodsWithResponse(cmd.Context(), projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list payment methods: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}

	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	return resp.JSON200.PaymentMethods, nil
}

func apiKeyPrefixCompletion(app *common.App) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		keys, err := listApiKeys(cmd, app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		results := make([]string, 0, len(keys))
		for _, key := range keys {
			if strings.HasPrefix(key.Prefix, toComplete) {
				results = append(results, cobra.CompletionWithDesc(key.Prefix, key.Name))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	}
}

func listApiKeys(cmd *cobra.Command, app *common.App) ([]api.ApiKey, error) {
	client, projectID, err := app.GetClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.ListApiKeysWithResponse(cmd.Context(), projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}

	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	return *resp.JSON200, nil
}

var validSizes = []string{"1x", "2x", "4x", "8x"}

var sizeCompletion = cobra.FixedCompletions(validSizes, cobra.ShellCompDirectiveNoFileComp)

func shareTokenCompletion(app *common.App) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Share token is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		client, projectID, err := app.GetClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		resp, err := client.ListSharesWithResponse(cmd.Context(), projectID)
		if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Skip revoked shares — they can't be revoked again, so suggesting
		// them as completions would just produce errors.
		results := make([]string, 0, len(*resp.JSON200))
		for _, share := range *resp.JSON200 {
			if share.RevokedAt != nil {
				continue
			}
			if strings.HasPrefix(share.ShareToken, toComplete) {
				results = append(results, cobra.CompletionWithDesc(share.ShareToken, share.DatabaseName))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	}
}

// filterCompletionsByPrefix filters a slice of strings to only include items
// that start with the given prefix. This is used by shell completion functions
// to narrow down suggestions based on what the user has typed so far.
func filterCompletionsByPrefix(items []string, prefix string) []string {
	var filtered []string
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
