package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/mcp"
	"github.com/timescale/ghost/internal/util"
)

// buildMCPListCmd creates the list subcommand for displaying available MCP capabilities
func buildMCPListCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available MCP tools, prompts, and resources",
		Long: `List all MCP tools, prompts, and resources exposed via the Ghost MCP server.

The output can be formatted as a table, JSON, or YAML.`,
		Aliases: []string{"ls"},
		Example: `  # List all capabilities in table format (default)
  ghost mcp list

  # List as JSON
  ghost mcp list --json

  # List as YAML
  ghost mcp list --yaml`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create MCP server
			server, err := mcp.NewServer(cmd.Context(), app, nil)
			if err != nil {
				return fmt.Errorf("failed to create MCP server: %w", err)
			}
			defer server.Close()

			// List capabilities
			capabilities, err := server.ListCapabilities(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list capabilities: %w", err)
			}

			// Close the MCP server when finished
			if err := server.Close(); err != nil {
				return fmt.Errorf("failed to close MCP server: %w", err)
			}

			// Format output
			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), capabilities)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), capabilities)
			default:
				return outputMCPList(cmd.OutOrStdout(), capabilities)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	return cmd
}

// outputMCPList outputs capabilities in text/table format. Results are ordered
// alphabetically by type, then name.
func outputMCPList(output io.Writer, capabilities *mcp.Capabilities) error {
	table := common.NewTable(output)

	table.Header("TYPE", "NAME")

	// Add prompts
	for _, prompt := range capabilities.Prompts {
		table.Append("prompt", prompt.Name)
	}

	// Add resources
	for _, resource := range capabilities.Resources {
		table.Append("resource", resource.Name)
	}

	// Add resource templates
	for _, template := range capabilities.ResourceTemplates {
		table.Append("resource_template", template.Name)
	}

	// Add tools
	for _, tool := range capabilities.Tools {
		table.Append("tool", tool.Name)
	}

	return table.Render()
}
