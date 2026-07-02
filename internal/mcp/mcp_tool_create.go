package mcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/util"
)

// MCPToolCreateInput represents input for ghost_mcp_tool_create
type MCPToolCreateInput struct {
	Ref   string `json:"name_or_id"`
	Name  string `json:"name"`
	Query string `json:"query"`
}

func (MCPToolCreateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolCreateInput](nil))
	databaseRefInputProperties(schema)
	queryToolNameInputProperties(schema)
	queryToolQueryInputProperties(schema)
	return schema
}

// MCPToolCreateOutput represents output for ghost_mcp_tool_create
type MCPToolCreateOutput struct {
	Message string `json:"message"`
}

func (MCPToolCreateOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[MCPToolCreateOutput](nil))
	schema.Properties["message"].Description = "Confirmation message"
	return schema
}

func newMCPToolCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "ghost_mcp_tool_create",
		Title: "Create Query Tool",
		Description: "Create a new custom MCP query tool on a database from a SQL query. " +
			"The query must include a sqlc directive of the form '-- name: <name> :<cmd>' (where <cmd> is one of :one, :many, or :exec) whose name matches the tool name. " +
			"The query is validated against the live database and rejected (with diagnostics) if invalid. " +
			"Fails if a tool with that name already exists on the database. " +
			"On success the tool list is updated immediately. " +
			"Use ghost_schema first to understand the available tables and columns.",
		InputSchema:  MCPToolCreateInput{}.Schema(),
		OutputSchema: MCPToolCreateOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			// Creation fails if the tool already exists, so it is additive.
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false,
			OpenWorldHint:   new(true),
			Title:           "Create Query Tool",
		},
	}
}

func (s *Server) handleMCPToolCreate(ctx context.Context, req *mcp.CallToolRequest, input MCPToolCreateInput) (*mcp.CallToolResult, MCPToolCreateOutput, error) {
	cfg := s.app.GetConfig()
	if err := checkReadOnly(cfg); err != nil {
		return nil, MCPToolCreateOutput{}, err
	}

	if err := s.queryManager.CreateTool(ctx, input.Ref, input.Name, input.Query); err != nil {
		return nil, MCPToolCreateOutput{}, handleDatabaseError(err)
	}

	return nil, MCPToolCreateOutput{
		Message: fmt.Sprintf("query tool %q created", input.Name),
	}, nil
}
