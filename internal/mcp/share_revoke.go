package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
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
		Description:  "Revoke a database share so its URL can no longer be used to create new forks.",
		InputSchema:  ShareRevokeInput{}.Schema(),
		OutputSchema: Share{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(true),
			Title:           "Revoke Database Share",
		},
	}
}

func (s *Server) handleShareRevoke(ctx context.Context, req *mcp.CallToolRequest, input ShareRevokeInput) (*mcp.CallToolResult, Share, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, Share{}, err
	}

	if err := checkReadOnly(cfg); err != nil {
		return nil, Share{}, err
	}

	share, err := findShareByToken(ctx, client, projectID, input.ShareToken)
	if err != nil {
		return nil, Share{}, err
	}

	resp, err := client.RevokeShareWithResponse(ctx, projectID, share.Id)
	if err != nil {
		return nil, Share{}, fmt.Errorf("failed to revoke share: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, Share{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return nil, Share{}, errors.New("empty response from API")
	}

	return nil, toShare(*resp.JSON200, cfg.ShareURL, time.Now()), nil
}

// findShareByToken looks up the API share matching the given token. The
// revoke API requires the internal share ID, so we list shares and match on
// token client-side to keep the ID out of the user-facing surface.
func findShareByToken(ctx context.Context, client api.ClientWithResponsesInterface, projectID, token string) (api.DatabaseShare, error) {
	resp, err := client.ListSharesWithResponse(ctx, projectID)
	if err != nil {
		return api.DatabaseShare{}, fmt.Errorf("failed to list shares: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return api.DatabaseShare{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return api.DatabaseShare{}, errors.New("empty response from API")
	}
	for _, s := range *resp.JSON200 {
		if s.ShareToken == token {
			return s, nil
		}
	}
	return api.DatabaseShare{}, fmt.Errorf("share not found for the given token")
}
