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

// withAppLoad wraps a completion function, loading the config and API client
// before invoking it. The app is only loaded automatically for wrapped
// commands (see wrapCommands), not for the __complete commands that drive
// live tab completion — so completions which don't need the config or client
// (subcommand names, flag names, static lists) avoid the config file, the
// system keyring, and the network. Completion functions that do need them
// must be wrapped with this helper.
func withAppLoad(app *common.App, fn cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		app.SetFlags(cmd.Flags())
		if _, _, _, err := app.Load(cmd.Context()); err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return fn(cmd, args, toComplete)
	}
}

func databaseCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	})
}

func spaceCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Space ID is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		client, _, err := app.GetClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		resp, err := client.ListSpacesWithResponse(cmd.Context())
		if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Suggest space IDs, with the space name as the description
		spaces := *resp.JSON200
		results := make([]string, 0, len(spaces))
		for _, space := range spaces {
			if strings.HasPrefix(space.Id, toComplete) {
				results = append(results, cobra.CompletionWithDesc(space.Id, space.Name))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

// memberEmailCompletion completes the email of a space member as the first
// positional argument.
func memberEmailCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		client, spaceID, err := app.GetClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		resp, err := client.ListMembersWithResponse(cmd.Context(), api.SpaceId(spaceID))
		if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Suggest member emails, with the member name as the description.
		// Emails are stored lowercase, so match the typed prefix
		// case-insensitively.
		members := *resp.JSON200
		results := make([]string, 0, len(members))
		for _, member := range members {
			if strings.HasPrefix(member.Email, strings.ToLower(toComplete)) {
				results = append(results, cobra.CompletionWithDesc(member.Email, member.Name))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

// grantableRoles are the roles that can be granted via invite or member-role
// changes, in privilege order. Owner is never grantable, so it's excluded.
var grantableRoles = []string{
	string(api.MemberRoleAdmin),
	string(api.MemberRoleDeveloper),
	string(api.MemberRoleViewer),
}

// memberEmailRoleCompletion completes the email of a space member as the first
// positional argument (via memberEmailCompletion), then the grantable role
// vocabulary as the second (for 'ghost member role'). The role branch is a
// static list and needs no app load; the email branch loads the app via
// memberEmailCompletion's own withAppLoad wrapper.
func memberEmailRoleCompletion(app *common.App) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 1 {
			return filterCompletionsByPrefix(grantableRoles, toComplete), cobra.ShellCompDirectiveNoFileComp
		}
		return memberEmailCompletion(app)(cmd, args, toComplete)
	}
}

// inviteSentCompletion completes a sent invite, suggesting the invitee email
// (with the granted role as the description) as the first positional argument.
func inviteSentCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		client, spaceID, err := app.GetClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		resp, err := client.ListInvitesWithResponse(cmd.Context(), api.SpaceId(spaceID))
		if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Emails are stored lowercase, so match the typed prefix
		// case-insensitively.
		invites := *resp.JSON200
		results := make([]string, 0, len(invites))
		for _, invite := range invites {
			if strings.HasPrefix(invite.Email, strings.ToLower(toComplete)) {
				results = append(results, cobra.CompletionWithDesc(invite.Email, string(invite.Role)))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

// inviteReceivedCompletion completes a received invitation, suggesting the
// space ID (with the space name as the description) as the first positional
// argument.
func inviteReceivedCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		client, _, err := app.GetClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		resp, err := client.ListReceivedInvitesWithResponse(cmd.Context())
		if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		invitations := *resp.JSON200
		results := make([]string, 0, len(invitations))
		for _, invitation := range invitations {
			if strings.HasPrefix(invitation.SpaceId, toComplete) {
				results = append(results, cobra.CompletionWithDesc(invitation.SpaceId, invitation.SpaceName))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

func listDatabases(cmd *cobra.Command, app *common.App) ([]api.DatabaseWithUsage, error) {
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

func mcpCapabilityCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Capability name is always first positional argument
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Create MCP server to get capabilities. Never connects to any
		// databases, so ManagementOnly rather than Enabled.
		functionTools := mcp.FunctionToolsDisabled
		if app.Experimental {
			functionTools = mcp.FunctionToolsManagementOnly
		}
		server, err := mcp.NewServer(cmd.Context(), app, mcp.Options{
			FunctionTools: functionTools,
		})
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
	})
}

func paymentMethodIDCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	})
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

func apiKeyPrefixCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	})
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

func invoiceIDCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		invoices, err := listInvoices(cmd, app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		results := make([]string, 0, len(invoices))
		for _, inv := range invoices {
			if strings.HasPrefix(inv.Id, toComplete) {
				desc := fmt.Sprintf("%s (%s)", inv.InvoiceNumber, inv.InvoiceDate.Format("2006-01-02"))
				results = append(results, cobra.CompletionWithDesc(inv.Id, desc))
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})
}

func listInvoices(cmd *cobra.Command, app *common.App) ([]api.Invoice, error) {
	client, projectID, err := app.GetClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.ListInvoicesWithResponse(cmd.Context(), projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list invoices: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}

	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	return resp.JSON200.Invoices, nil
}

var validSizes = []string{"1x", "2x", "4x", "8x"}

var sizeCompletion = cobra.FixedCompletions(validSizes, cobra.ShellCompDirectiveNoFileComp)

var logLevelCompletion = cobra.FixedCompletions([]string{"debug", "info", "warn", "error"}, cobra.ShellCompDirectiveNoFileComp)

func shareTokenCompletion(app *common.App) cobra.CompletionFunc {
	return withAppLoad(app, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	})
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
