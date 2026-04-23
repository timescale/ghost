package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// CreateInput represents input for ghost_create
type CreateInput struct {
	Name       string `json:"name,omitempty"`
	ShareToken string `json:"share_token,omitempty"`
	Wait       bool   `json:"wait,omitempty"`
}

func (CreateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[CreateInput](nil))
	createNameInputProperties(schema)
	shareTokenInputProperties(schema)
	waitInputProperties(schema)
	return schema
}

// CreateOutput represents output for ghost_create
type CreateOutput struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	ConnectionString string   `json:"connection_string"`
	Warnings         []string `json:"warnings,omitempty"`
}

func (CreateOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[CreateOutput](nil))
	databaseIDOutputProperties(schema)
	databaseNameOutputProperties(schema)
	connectionStringOutputProperties(schema)
	warningsOutputProperties(schema)
	return schema
}

func newCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "ghost_create",
		Title: "Create Database",
		Description: `Create a new database.

Note: new databases may take a few minutes to start up. Use ghost_list to check the current status.`,
		InputSchema:  CreateInput{}.Schema(),
		OutputSchema: CreateOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false,
			OpenWorldHint:   new(true),
			Title:           "Create Database",
		},
	}
}

func (s *Server) handleCreate(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, CreateOutput, error) {
	result, err := s.createDatabase(ctx, createDatabaseArgs{
		req: api.CreateDatabaseRequest{
			Name:       util.PtrIfNonZero(input.Name),
			ShareToken: util.PtrIfNonZero(input.ShareToken),
		},
		wait: input.Wait,
	})
	if err != nil {
		return nil, CreateOutput{}, err
	}

	return nil, CreateOutput{
		ID:               result.id,
		Name:             result.name,
		ConnectionString: result.connectionString,
		Warnings:         result.warnings,
	}, nil
}

type createDatabaseArgs struct {
	req  api.CreateDatabaseRequest
	wait bool
}

type createDatabaseResult struct {
	id               string
	name             string
	size             string
	connectionString string
	warnings         []string
}

// createDatabase is a shared helper for ghost_create and ghost_create_dedicated.
func (s *Server) createDatabase(ctx context.Context, args createDatabaseArgs) (createDatabaseResult, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return createDatabaseResult{}, err
	}

	// Make API call to create database
	// API defaults all values based on Ghost project plan type
	resp, err := client.CreateDatabaseWithResponse(ctx, projectID, args.req)
	if err != nil {
		return createDatabaseResult{}, fmt.Errorf("failed to create database: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusAccepted {
		return createDatabaseResult{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}

	if resp.JSON202 == nil {
		return createDatabaseResult{}, errors.New("empty response from API")
	}
	database := *resp.JSON202
	databaseID := database.Id

	// Save password to .pgpass file
	var warnings []string
	password := util.Deref(database.Password)
	if password == "" {
		warnings = append(warnings, "no initial password returned by API")
	} else {
		if err := common.SavePassword(database, password, "tsdbadmin"); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to save password to .pgpass: %v", err))
		}
	}

	// Build connection string using InitialPassword directly
	connStr, err := common.BuildConnectionString(common.ConnectionStringArgs{
		Database: database,
		Role:     "tsdbadmin",
		Password: password,
	})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to build connection string: %v", err))
	}

	if args.wait {
		if err := common.WaitForDatabase(ctx, common.WaitForDatabaseArgs{
			Client:      client,
			ProjectID:   projectID,
			DatabaseRef: databaseID,
		}); err != nil {
			return createDatabaseResult{}, err
		}
	}

	// TODO: use API response size when available instead of request size
	return createDatabaseResult{
		id:               databaseID,
		name:             database.Name,
		size:             util.DerefStr(args.req.Size),
		connectionString: connStr,
		warnings:         warnings,
	}, nil
}
