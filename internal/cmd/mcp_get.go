package cmd

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/spf13/cobra"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/mcp"
	"github.com/timescale/ghost/internal/util"
)

// buildMCPGetCmd creates the get subcommand for displaying detailed info on a specific MCP capability
func buildMCPGetCmd(app *common.App) *cobra.Command {
	var jsonOutput bool
	var yamlOutput bool
	var functionTools bool

	cmd := &cobra.Command{
		Use:     "get <name>",
		Aliases: []string{"describe", "show"},
		Short:   "Get detailed information about a specific MCP capability",
		Long:    `Get detailed information about a specific MCP tool, prompt, resource, or resource template.`,
		Example: `  # Get details about a tool
  ghost mcp get ghost_create

  # Get details about a prompt
  ghost mcp get setup-timescaledb-hypertables

  # Get details as JSON
  ghost mcp get ghost_create --json

  # Get details as YAML
  ghost mcp get ghost_create --yaml`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: mcpCapabilityCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			capabilityName := args[0]

			// Create MCP server (see functionToolsMode for the --function-tools gating).
			server, err := mcp.NewServer(cmd.Context(), app, mcp.Options{
				FunctionTools: functionToolsMode(app, functionTools),
			})
			if err != nil {
				return fmt.Errorf("failed to create MCP server: %w", err)
			}
			defer server.Close()

			// List all capabilities
			capabilities, err := server.ListCapabilities(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list capabilities: %w", err)
			}

			// Close the MCP server when finished
			if err := server.Close(); err != nil {
				return fmt.Errorf("failed to close MCP server: %w", err)
			}

			// Find the specific capability
			capability := capabilities.Get(capabilityName)
			if capability == nil {
				return fmt.Errorf("capability %q not found", capabilityName)
			}

			// Format output
			switch {
			case jsonOutput:
				return util.SerializeToJSON(cmd.OutOrStdout(), capability)
			case yamlOutput:
				return util.SerializeToYAML(cmd.OutOrStdout(), capability)
			default:
				switch c := capability.(type) {
				case *mcpsdk.Tool:
					return outputMCPTool(cmd, c)
				case *mcpsdk.Prompt:
					return outputMCPPrompt(cmd, c)
				case *mcpsdk.Resource:
					return outputMCPResource(cmd, c)
				case *mcpsdk.ResourceTemplate:
					return outputMCPResourceTemplate(cmd, c)
				default:
					return fmt.Errorf("unsupported capability type: %T", c)
				}
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&yamlOutput, "yaml", false, "Output in YAML format")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")
	addFunctionToolsFlag(cmd, app, &functionTools)

	return cmd
}

// outputMCPTool outputs a tool in text format
func outputMCPTool(cmd *cobra.Command, tool *mcpsdk.Tool) error {
	var lines []string

	// Title line with annotation tags
	titleLine := tool.Title
	if titleLine == "" {
		titleLine = tool.Name
	}

	// Add annotation tags to title (each in separate brackets)
	if tool.Annotations != nil {
		var tags []string
		ann := tool.Annotations

		if ann.ReadOnlyHint {
			tags = append(tags, "[read-only]")
		}
		if !ann.ReadOnlyHint && ann.IdempotentHint {
			tags = append(tags, "[idempotent]")
		}
		if !ann.ReadOnlyHint && ann.DestructiveHint != nil && *ann.DestructiveHint {
			tags = append(tags, "[destructive]")
		}
		if ann.OpenWorldHint != nil && *ann.OpenWorldHint {
			tags = append(tags, "[open-world]")
		}

		if len(tags) > 0 {
			titleLine += " " + strings.Join(tags, " ")
		}
	}

	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Tool name
	lines = append(lines, "Tool name: "+tool.Name)
	lines = append(lines, "")

	// Description
	if tool.Description != "" {
		lines = append(lines, "Description:")
		lines = append(lines, tool.Description)
		lines = append(lines, "")
	}

	// Parameters (input schema)
	if tool.InputSchema != nil {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return fmt.Errorf("error marshaling input schema to JSON: %w", err)
		}

		var inputSchema *jsonschema.Schema
		if err := json.Unmarshal(raw, &inputSchema); err != nil {
			return fmt.Errorf("error unmarshaling input schema from JSON: %w", err)
		}

		formatted := formatJSONSchema(inputSchema, 1)
		if formatted != "" {
			lines = append(lines, "Parameters:")
			lines = append(lines, formatted)
			lines = append(lines, "")
		}
	}

	// Output schema
	if tool.OutputSchema != nil {
		raw, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			return fmt.Errorf("error marshaling output schema to JSON: %w", err)
		}

		var outputSchema *jsonschema.Schema
		if err := json.Unmarshal(raw, &outputSchema); err != nil {
			return fmt.Errorf("error unmarshaling output schema from JSON: %w", err)
		}

		formatted := formatJSONSchema(outputSchema, 1)
		if formatted != "" {
			lines = append(lines, "Output:")
			lines = append(lines, formatted)
			lines = append(lines, "")
		}
	}

	// Write output
	cmd.Println(strings.Join(lines, "\n"))
	return nil
}

// outputMCPPrompt outputs a prompt in text format
func outputMCPPrompt(cmd *cobra.Command, prompt *mcpsdk.Prompt) error {
	var lines []string

	// Title line
	titleLine := prompt.Title
	if titleLine == "" {
		titleLine = prompt.Name
	}

	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Prompt name
	lines = append(lines, "Prompt name: "+prompt.Name)
	lines = append(lines, "")

	// Description
	if prompt.Description != "" {
		lines = append(lines, "Description:")
		lines = append(lines, prompt.Description)
		lines = append(lines, "")
	}

	// Arguments (formatted as bullet list)
	if len(prompt.Arguments) > 0 {
		lines = append(lines, "Arguments:")
		lines = append(lines, formatPromptArguments(prompt.Arguments))
		lines = append(lines, "")
	}

	// Write output
	cmd.Println(strings.Join(lines, "\n"))
	return nil
}

// outputMCPResource outputs a resource in text format
func outputMCPResource(cmd *cobra.Command, resource *mcpsdk.Resource) error {
	var lines []string

	// Title line
	titleLine := resource.Title
	if titleLine == "" {
		titleLine = resource.Name
	}

	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Resource name
	lines = append(lines, "Resource name: "+resource.Name)
	lines = append(lines, "")

	// Description
	if resource.Description != "" {
		lines = append(lines, "Description:")
		lines = append(lines, resource.Description)
		lines = append(lines, "")
	}

	// URI
	lines = append(lines, "URI: "+resource.URI)
	lines = append(lines, "")

	// Optional fields
	if resource.MIMEType != "" {
		lines = append(lines, "MIME Type: "+resource.MIMEType)
		lines = append(lines, "")
	}

	if resource.Size > 0 {
		lines = append(lines, fmt.Sprintf("Size: %d bytes", resource.Size))
		lines = append(lines, "")
	}

	// Annotations
	if resource.Annotations != nil {
		var annotations []string
		ann := resource.Annotations

		if len(ann.Audience) > 0 {
			audiences := make([]string, len(ann.Audience))
			for i, role := range ann.Audience {
				audiences[i] = string(role)
			}
			annotations = append(annotations, fmt.Sprintf("  • Audience: %v", audiences))
		}
		if ann.Priority != 0 {
			annotations = append(annotations, fmt.Sprintf("  • Priority: %f", ann.Priority))
		}
		if ann.LastModified != "" {
			annotations = append(annotations, "  • Last Modified: "+ann.LastModified)
		}

		if len(annotations) > 0 {
			lines = append(lines, "Annotations:")
			lines = append(lines, annotations...)
			lines = append(lines, "")
		}
	}

	// Write output
	cmd.Println(strings.Join(lines, "\n"))
	return nil
}

// outputMCPResourceTemplate outputs a resource template in text format
func outputMCPResourceTemplate(cmd *cobra.Command, template *mcpsdk.ResourceTemplate) error {
	var lines []string

	// Title line
	titleLine := template.Title
	if titleLine == "" {
		titleLine = template.Name
	}

	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Resource template name
	lines = append(lines, "Resource template name: "+template.Name)
	lines = append(lines, "")

	// Description
	if template.Description != "" {
		lines = append(lines, "Description:")
		lines = append(lines, template.Description)
		lines = append(lines, "")
	}

	// URI Template
	lines = append(lines, "URI Template: "+template.URITemplate)
	lines = append(lines, "")

	// Optional fields
	if template.MIMEType != "" {
		lines = append(lines, "MIME Type: "+template.MIMEType)
		lines = append(lines, "")
	}

	// Annotations
	if template.Annotations != nil {
		var annotations []string
		ann := template.Annotations

		if len(ann.Audience) > 0 {
			audiences := make([]string, len(ann.Audience))
			for i, role := range ann.Audience {
				audiences[i] = string(role)
			}
			annotations = append(annotations, fmt.Sprintf("  • Audience: %v", audiences))
		}
		if ann.Priority != 0 {
			annotations = append(annotations, fmt.Sprintf("  • Priority: %f", ann.Priority))
		}
		if ann.LastModified != "" {
			annotations = append(annotations, "  • Last Modified: "+ann.LastModified)
		}

		if len(annotations) > 0 {
			lines = append(lines, "Annotations:")
			lines = append(lines, annotations...)
			lines = append(lines, "")
		}
	}

	// Write output
	cmd.Println(strings.Join(lines, "\n"))
	return nil
}

// formatJSONSchema formats a JSON schema into a readable parameter list
func formatJSONSchema(s *jsonschema.Schema, indent int) string {
	if s == nil || len(s.Properties) == 0 {
		return ""
	}

	// Build formatted output
	indentStr := strings.Repeat("  ", indent)

	// Get property names and sort them alphabetically
	propNames := make([]string, 0, len(s.Properties))
	for propName := range s.Properties {
		propNames = append(propNames, propName)
	}
	slices.Sort(propNames)

	var lines []string
	for _, propName := range propNames {
		prop := s.Properties[propName]
		if prop == nil {
			continue
		}

		// Build property line with bullet point
		line := indentStr + "• " + propName

		// Add required marker
		if slices.Contains(s.Required, propName) {
			line += " (required)"
		}

		// Add type using recursive formatter
		if typeStr := formatSchemaType(prop); typeStr != "" && typeStr != "any" {
			line += ": " + typeStr
		}

		// Add description
		if prop.Description != "" {
			line += " - " + prop.Description
		}

		// Add default value
		if len(prop.Default) > 0 {
			line += " (default: " + string(prop.Default) + ")"
		}

		lines = append(lines, line)

		if len(prop.Properties) > 0 {
			// Handle nested objects
			nested := formatJSONSchema(prop, indent+1)
			if nested != "" {
				lines = append(lines, nested)
			}
		} else if prop.Items != nil && len(prop.Items.Properties) > 0 {
			// Handle nested arrays of objects
			nested := formatJSONSchema(prop.Items, indent+1)
			if nested != "" {
				lines = append(lines, nested)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// formatSchemaType recursively formats a JSON schema type into TypeScript-style syntax
func formatSchemaType(prop *jsonschema.Schema) string {
	if prop == nil {
		return ""
	}

	// Handle union types
	if len(prop.Types) > 0 {
		var types []string
		var hasNull bool
		for _, t := range prop.Types {
			if t == "array" && prop.Items != nil {
				// Recursively format array items
				itemType := formatSchemaType(prop.Items)
				if itemType == "" {
					itemType = "any"
				}
				types = append(types, "[]"+itemType)
			} else if t == "null" {
				hasNull = true
			} else {
				types = append(types, t)
			}
		}
		// Put null type at end
		if hasNull {
			types = append(types, "null")
		}
		return strings.Join(types, ", ")
	}

	// Handle single type
	if prop.Type == "array" && prop.Items != nil {
		// Recursively format array items
		itemType := formatSchemaType(prop.Items)
		if itemType == "" {
			itemType = "any"
		}
		return "[]" + itemType
	}

	// Return the base type, or "any" if no type is specified
	if prop.Type != "" {
		return prop.Type
	}
	return "any"
}

// formatPromptArguments formats prompt arguments into a readable bullet-point list
func formatPromptArguments(arguments []*mcpsdk.PromptArgument) string {
	if len(arguments) == 0 {
		return ""
	}

	// Sort arguments alphabetically by name
	sortedArgs := slices.Clone(arguments)
	slices.SortFunc(sortedArgs, func(a, b *mcpsdk.PromptArgument) int {
		return strings.Compare(a.Name, b.Name)
	})

	var lines []string
	for _, arg := range sortedArgs {
		// Build argument line with bullet point (2-space indent to match schema formatting)
		line := "  • " + arg.Name

		// Add required marker
		if arg.Required {
			line += " (required)"
		}

		// Add description
		if arg.Description != "" {
			line += " - " + arg.Description
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
