package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
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
