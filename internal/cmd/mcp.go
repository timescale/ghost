package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/mcp"
)

// buildMCPCmd creates the MCP server command with subcommands
func buildMCPCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Ghost Model Context Protocol (MCP) server",
		Long: `Ghost Model Context Protocol (MCP) server for AI assistant integration.

Exposes Ghost CLI functionality as MCP tools for Claude and other AI assistants.`,
	}

	// Add subcommands
	cmd.AddCommand(buildMCPInstallCmd(app))
	cmd.AddCommand(buildMCPUninstallCmd(app))
	cmd.AddCommand(buildMCPStatusCmd(app))
	cmd.AddCommand(buildMCPStartCmd(app))
	cmd.AddCommand(buildMCPListCmd(app))
	cmd.AddCommand(buildMCPGetCmd(app))

	return cmd
}

// addFunctionToolsFlag registers the --function-tools flag, shared by `mcp
// list` and `mcp get`: it additionally includes each database's generated
// (@mcp) function tools in the listing, by connecting to and introspecting
// every database in the space rather than just registering the built-in
// ghost_* tools and the refresh management tool. Like the rest of the
// function-tool feature, it is experimental.
func addFunctionToolsFlag(cmd *cobra.Command, app *common.App, functionTools *bool) {
	if !app.Experimental {
		return
	}
	cmd.Flags().BoolVar(functionTools, "function-tools", false,
		"Also include each database's generated custom function tools (connects to every database in the space)")
}

// functionToolsMode picks the function-tool mode for `mcp list`/`mcp get`:
// functionTools (only ever settable when app.Experimental — see
// addFunctionToolsFlag) enables the full feature; otherwise the refresh
// management tool is still registered when experimental, so listings stay
// accurate, but no database is connected to.
func functionToolsMode(app *common.App, functionTools bool) mcp.FunctionToolsMode {
	switch {
	case functionTools:
		return mcp.FunctionToolsEnabled
	case app.Experimental:
		return mcp.FunctionToolsManagementOnly
	default:
		return mcp.FunctionToolsDisabled
	}
}
