package mcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/util"
)

// MCPToolDeleteInput represents input for ghost_mcp_tool_delete
type MCPToolDeleteInput struct {
	Ref  string `json:"name_or_id"`
	Name string `json:"name"`
}

func (MCPToolDeleteInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolDeleteInput](nil))
	databaseRefInputProperties(schema)
	queryToolNameInputProperties(schema)
	return schema
}

// MCPToolDeleteOutput represents output for ghost_mcp_tool_delete
type MCPToolDeleteOutput struct {
	Message string `json:"message"`
}

func (MCPToolDeleteOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolDeleteOutput](nil))
	schema.Properties["message"].Description = "Confirmation message"
	return schema
}

func newMCPToolDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_mcp_tool_delete",
		Title:        "Delete Query Tool",
		Description:  "Remove an existing custom MCP query tool from a database by name, deleting its backing SQL. On success the tool list is updated immediately.",
		InputSchema:  MCPToolDeleteInput{}.Schema(),
		OutputSchema: MCPToolDeleteOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(true),
			Title:           "Delete Query Tool",
		},
	}
}

func (s *Server) handleMCPToolDelete(ctx context.Context, req *mcp.CallToolRequest, input MCPToolDeleteInput) (*mcp.CallToolResult, MCPToolDeleteOutput, error) {
	cfg := s.app.GetConfig()
	if err := checkReadOnly(cfg); err != nil {
		return nil, MCPToolDeleteOutput{}, err
	}

	if err := s.queryManager.DeleteTool(ctx, input.Ref, input.Name); err != nil {
		return nil, MCPToolDeleteOutput{}, handleDatabaseError(err)
	}

	return nil, MCPToolDeleteOutput{
		Message: fmt.Sprintf("query tool %q removed", input.Name),
	}, nil
}
