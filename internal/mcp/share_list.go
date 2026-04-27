package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// ShareListInput represents input for ghost_share_list (no parameters).
type ShareListInput struct{}

func (ShareListInput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ShareListInput](nil))
}

// ShareListOutput represents output for ghost_share_list.
type ShareListOutput struct {
	Shares []ShareOutput `json:"shares"`
}

func (ShareListOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ShareListOutput](nil))
	shareOutputProperties(schema.Properties["shares"].Items)
	return schema
}

func newShareListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_share_list",
		Title:        "List Database Shares",
		Description:  "List all shares in the current Ghost space.",
		InputSchema:  ShareListInput{}.Schema(),
		OutputSchema: ShareListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
			Title:         "List Database Shares",
		},
	}
}

func (s *Server) handleShareList(ctx context.Context, req *mcp.CallToolRequest, input ShareListInput) (*mcp.CallToolResult, ShareListOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ShareListOutput{}, err
	}

	resp, err := client.ListSharesWithResponse(ctx, projectID)
	if err != nil {
		return nil, ShareListOutput{}, fmt.Errorf("failed to list shares: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, ShareListOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return nil, ShareListOutput{}, errors.New("empty response from API")
	}
	shares := *resp.JSON200

	now := time.Now()
	output := ShareListOutput{
		Shares: make([]ShareOutput, len(shares)),
	}
	for i, sh := range shares {
		share, err := toShareOutput(sh, cfg.ShareURL, now)
		if err != nil {
			return nil, ShareListOutput{}, err
		}
		output.Shares[i] = share
	}

	return nil, output, nil
}
