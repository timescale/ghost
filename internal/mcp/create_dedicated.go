package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/util"
)

// CreateDedicatedInput represents input for ghost_create_dedicated
type CreateDedicatedInput struct {
	Name       string `json:"name,omitempty"`
	Size       string `json:"size,omitempty"`
	ShareToken string `json:"share_token,omitempty"`
	Wait       bool   `json:"wait,omitempty"`
}

func (CreateDedicatedInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[CreateDedicatedInput](nil))
	createNameInputProperties(schema)
	sizeInputProperties(schema)
	shareTokenInputProperties(schema)
	waitInputProperties(schema)
	return schema
}

// CreateDedicatedOutput represents output for ghost_create_dedicated
type CreateDedicatedOutput struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Size             string   `json:"size"`
	ConnectionString string   `json:"connection_string"`
	Warnings         []string `json:"warnings,omitempty"`
}

func (CreateDedicatedOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[CreateDedicatedOutput](nil))
	databaseIDOutputProperties(schema)
	databaseNameOutputProperties(schema)
	sizeOutputProperties(schema)
	connectionStringOutputProperties(schema)
	warningsOutputProperties(schema)
	return schema
}

func newCreateDedicatedTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "ghost_create_dedicated",
		Title: "Create Dedicated Database",
		Description: `Create a new dedicated database. Dedicated databases are always-on,
billed instances that are not subject to space compute or storage limits.
A payment method must be on file.

Note: new databases may take a few minutes to start up. Use ghost_list to check the current status.`,
		InputSchema:  CreateDedicatedInput{}.Schema(),
		OutputSchema: CreateDedicatedOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false,
			OpenWorldHint:   new(true),
			Title:           "Create Dedicated Database",
		},
	}
}

func (s *Server) handleCreateDedicated(ctx context.Context, req *mcp.CallToolRequest, input CreateDedicatedInput) (*mcp.CallToolResult, CreateDedicatedOutput, error) {
	result, err := s.createDatabase(ctx, createDatabaseArgs{
		req: api.CreateDatabaseRequest{
			Name:       util.PtrIfNonZero(input.Name),
			Type:       new(api.DatabaseTypeDedicated),
			Size:       new(api.DatabaseSize(input.Size)),
			ShareToken: util.PtrIfNonZero(input.ShareToken),
		},
		wait: input.Wait,
	})
	if err != nil {
		return nil, CreateDedicatedOutput{}, err
	}

	return nil, CreateDedicatedOutput{
		ID:               result.id,
		Name:             result.name,
		Size:             result.size,
		ConnectionString: result.connectionString,
		Warnings:         result.warnings,
	}, nil
}
