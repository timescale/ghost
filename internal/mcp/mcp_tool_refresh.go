package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/util"
)

// MCPToolRefreshInput represents input for ghost_mcp_tool_refresh
type MCPToolRefreshInput struct {
	Ref string `json:"name_or_id"`
}

func (MCPToolRefreshInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolRefreshInput](nil))
	databaseRefInputProperties(schema)
	return schema
}

// MCPToolRefreshOutput represents output for ghost_mcp_tool_refresh
type MCPToolRefreshOutput struct {
	Tools []string `json:"tools"`
}

func (MCPToolRefreshOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolRefreshOutput](nil))
	schema.Properties["tools"].Description = "The database's registered function tool names after the refresh"
	return schema
}

func newMCPToolRefreshTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "ghost_mcp_tool_refresh",
		Title: "Refresh Function Tools",
		Description: "Re-introspect a database's @mcp-marked functions and update its generated MCP tools, picking up functions created, changed, or dropped since the last refresh (or server start). " +
			"Call this after creating or modifying @mcp functions (e.g. via ghost_sql) to make the corresponding tools available immediately. " +
			"Returns the database's current tool list.",
		InputSchema:  MCPToolRefreshInput{}.Schema(),
		OutputSchema: MCPToolRefreshOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			// Refreshing only reads the catalog; it changes the server's
			// registered tools, not the database.
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  new(true),
			Title:          "Refresh Function Tools",
		},
	}
}

func (s *Server) handleMCPToolRefresh(ctx context.Context, req *mcp.CallToolRequest, input MCPToolRefreshInput) (*mcp.CallToolResult, MCPToolRefreshOutput, error) {
	tools, err := s.functionManager.Load(ctx, input.Ref)
	if err != nil {
		return nil, MCPToolRefreshOutput{}, handleDatabaseError(err)
	}
	if tools == nil {
		// Serialize an empty tool list as [], not null.
		tools = []string{}
	}

	return nil, MCPToolRefreshOutput{
		Tools: tools,
	}, nil
}
