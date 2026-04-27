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

// ShareRevokeInput represents input for ghost_share_revoke.
type ShareRevokeInput struct {
	ShareToken string `json:"share_token"`
}

func (ShareRevokeInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ShareRevokeInput](nil))
	schema.Properties["share_token"].Description = "Share token to revoke (as returned by ghost_share or ghost_share_list)"
	return schema
}

func newShareRevokeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_share_revoke",
		Title:        "Revoke Database Share",
		Description:  "Revoke a database share so its URL can no longer be used to create new databases.",
		InputSchema:  ShareRevokeInput{}.Schema(),
		OutputSchema: ShareOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(true),
			Title:           "Revoke Database Share",
		},
	}
}

func (s *Server) handleShareRevoke(ctx context.Context, req *mcp.CallToolRequest, input ShareRevokeInput) (*mcp.CallToolResult, ShareOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ShareOutput{}, err
	}

	if err := checkReadOnly(cfg); err != nil {
		return nil, ShareOutput{}, err
	}

	resp, err := client.RevokeShareWithResponse(ctx, projectID, input.ShareToken)
	if err != nil {
		return nil, ShareOutput{}, fmt.Errorf("failed to revoke share: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, ShareOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return nil, ShareOutput{}, errors.New("empty response from API")
	}

	output, err := toShareOutput(*resp.JSON200, cfg.ShareURL, time.Now())
	if err != nil {
		return nil, ShareOutput{}, err
	}
	return nil, output, nil
}
