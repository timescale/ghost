package mcp

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// SchemaInput represents input for ghost_schema
type SchemaInput struct {
	Ref         string `json:"name_or_id"`
	SchemaName  string `json:"schema,omitempty"`
	Internal    bool   `json:"internal,omitempty"`
	Definitions bool   `json:"definitions,omitempty"`
	Comments    bool   `json:"comments,omitempty"`
}

func (SchemaInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[SchemaInput](nil))
	databaseRefInputProperties(schema)
	schema.Properties["schema"].Description = "Restrict output to a single Postgres schema. Only objects the connecting user can access are returned."
	schema.Properties["schema"].Examples = []any{"public", "reporting", "pg_catalog"}
	schema.Properties["internal"].Description = "Include system schemas (information_schema, pg_*, _timescaledb_*) and extension-owned objects. Defaults to false."
	schema.Properties["internal"].Default = json.RawMessage("false")
	schema.Properties["definitions"].Description = "Include full object definitions (view SELECT statements and function/procedure bodies). Omitted by default to keep the output concise. Defaults to false."
	schema.Properties["definitions"].Default = json.RawMessage("false")
	schema.Properties["comments"].Description = "Include object comments (COMMENT ON text for schemas, tables, views, columns, enums, functions, and procedures). Omitted by default to keep the output concise. Defaults to false."
	schema.Properties["comments"].Default = json.RawMessage("false")
	return schema
}

func newSchemaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "ghost_schema",
		Title:       "Show Database Schema",
		Description: "Display database schema including tables (regular, partitioned, and foreign), views, materialized views, enum types, functions, procedures, indexes, triggers, and TimescaleDB hypertable and continuous aggregate metadata. Only objects the connecting user can access are returned.",
		InputSchema: SchemaInput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
			Title:         "Show Database Schema",
		},
	}
}

func (s *Server) handleSchema(ctx context.Context, req *mcp.CallToolRequest, input SchemaInput) (*mcp.CallToolResult, any, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, nil, err
	}

	schema, err := common.FetchDatabaseSchema(ctx, common.FetchDatabaseSchemaArgs{
		Client:             client,
		ProjectID:          projectID,
		DatabaseRef:        input.Ref,
		Schema:             input.SchemaName,
		IncludeInternal:    input.Internal,
		IncludeDefinitions: input.Definitions,
		IncludeComments:    input.Comments,
	})
	if err != nil {
		return nil, nil, handleDatabaseError(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: common.FormatSchema(schema)},
		},
	}, nil, nil
}
