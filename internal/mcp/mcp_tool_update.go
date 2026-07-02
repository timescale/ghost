package mcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/util"
)

// MCPToolUpdateInput represents input for ghost_mcp_tool_update
type MCPToolUpdateInput struct {
	Ref   string `json:"name_or_id"`
	Name  string `json:"name"`
	Query string `json:"query"`
}

func (MCPToolUpdateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolUpdateInput](nil))
	databaseRefInputProperties(schema)
	queryToolNameInputProperties(schema)
	queryToolQueryInputProperties(schema)
	return schema
}

// MCPToolUpdateOutput represents output for ghost_mcp_tool_update
type MCPToolUpdateOutput struct {
	Message string `json:"message"`
}

func (MCPToolUpdateOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolUpdateOutput](nil))
	schema.Properties["message"].Description = "Confirmation message"
	return schema
}

func newMCPToolUpdateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "ghost_mcp_tool_update",
		Title: "Update Query Tool",
		Description: "Replace the SQL backing an existing custom MCP query tool on a database. " +
			"The query must include a sqlc directive of the form '-- name: <name> :<cmd>' (where <cmd> is one of :one, :many, or :exec) whose name matches the tool name. " +
			"The query is validated against the live database and rejected (with diagnostics) if invalid. " +
			"Fails if no tool with that name exists on the database. " +
			"On success the tool list is updated immediately.",
		InputSchema:  MCPToolUpdateInput{}.Schema(),
		OutputSchema: MCPToolUpdateOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(true),
			Title:           "Update Query Tool",
		},
	}
}

func (s *Server) handleMCPToolUpdate(ctx context.Context, req *mcp.CallToolRequest, input MCPToolUpdateInput) (*mcp.CallToolResult, MCPToolUpdateOutput, error) {
	cfg := s.app.GetConfig()
	if err := checkReadOnly(cfg); err != nil {
		return nil, MCPToolUpdateOutput{}, err
	}

	if err := s.queryManager.UpdateTool(ctx, input.Ref, input.Name, input.Query); err != nil {
		return nil, MCPToolUpdateOutput{}, handleDatabaseError(err)
	}

	return nil, MCPToolUpdateOutput{
		Message: fmt.Sprintf("query tool %q updated", input.Name),
	}, nil
}
