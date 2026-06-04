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

// IDInput represents input for ghost_id (empty - no parameters)
type IDInput struct{}

func (IDInput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[IDInput](nil))
}

// IDOutput is the MCP tool's output type. It has the same underlying type as
// api.AuthInfo so values convert directly, and is redeclared here so the tool
// can attach a Schema() method (matching the pattern other MCP tools use).
type IDOutput api.AuthInfo

func (IDOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[IDOutput](nil))
	schema.Properties["type"].Description = "The authentication method used."
	schema.Properties["type"].Enum = []any{"user", "api_key"}
	schema.Properties["user"].Description = "Details of the authenticated user. Present when authenticating as a user (e.g. via OAuth login)."
	schema.Properties["api_key"].Description = "Details of the API key used for authentication, including the user who created it and the space it is scoped to. Present when authenticating with an API key."
	return schema
}

func newIDTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_id",
		Title:        "Show Identity",
		Description:  "Display information about the authenticated caller: the user when authenticating via OAuth, or the API key when authenticating with an API key.",
		InputSchema:  IDInput{}.Schema(),
		OutputSchema: IDOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
			Title:         "Show Identity",
		},
	}
}

func (s *Server) handleID(ctx context.Context, req *mcp.CallToolRequest, input IDInput) (*mcp.CallToolResult, IDOutput, error) {
	client, _, err := s.app.GetClient()
	if err != nil {
		return nil, IDOutput{}, err
	}

	resp, err := client.AuthInfoWithResponse(ctx)
	if err != nil {
		return nil, IDOutput{}, fmt.Errorf("failed to get auth info: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, IDOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return nil, IDOutput{}, errors.New("empty response from API")
	}

	return nil, IDOutput(*resp.JSON200), nil
}
