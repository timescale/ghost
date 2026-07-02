package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/util"
)

// MCPToolGetInput represents input for ghost_mcp_tool_get
type MCPToolGetInput struct {
	Ref  string `json:"name_or_id"`
	Name string `json:"name"`
}

func (MCPToolGetInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolGetInput](nil))
	databaseRefInputProperties(schema)
	queryToolNameInputProperties(schema)
	return schema
}

// MCPToolGetOutput represents output for ghost_mcp_tool_get
type MCPToolGetOutput struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}

func (MCPToolGetOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolGetOutput](nil))
	schema.Properties["name"].Description = "Name of the query tool"
	schema.Properties["query"].Description = "The SQL backing the tool, including its sqlc directive and comments"
	return schema
}

func newMCPToolGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_mcp_tool_get",
		Title:        "Get Query Tool",
		Description:  "Return the SQL query (including its sqlc directive and documentation comments) that backs an existing custom MCP query tool on a database.",
		InputSchema:  MCPToolGetInput{}.Schema(),
		OutputSchema: MCPToolGetOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
			Title:         "Get Query Tool",
		},
	}
}

func (s *Server) handleMCPToolGet(ctx context.Context, req *mcp.CallToolRequest, input MCPToolGetInput) (*mcp.CallToolResult, MCPToolGetOutput, error) {
	stored, err := s.queryManager.GetTool(ctx, input.Ref, input.Name)
	if err != nil {
		return nil, MCPToolGetOutput{}, handleDatabaseError(err)
	}

	return nil, MCPToolGetOutput{
		Name:  stored.Name,
		Query: stored.SQL,
	}, nil
}
