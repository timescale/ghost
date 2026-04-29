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

// ShareInput represents input for ghost_share.
type ShareInput struct {
	Ref     string `json:"name_or_id"`
	Expires string `json:"expires,omitempty"`
}

func (ShareInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ShareInput](nil))
	databaseRefInputProperties(schema)
	schema.Properties["expires"].Description = "Optional share expiry. Accepts a Go duration relative to now (e.g. \"30m\", \"24h\") or an RFC3339 timestamp (e.g. \"2026-05-01T00:00:00Z\"). If omitted, the share does not expire."
	schema.Properties["expires"].Examples = []any{"24h", "2026-05-01T00:00:00Z"}
	return schema
}

// ShareOutput represents a database share in MCP output. The share_token (also
// embedded in the URL) is the only identifier exposed — it's what a recipient
// passes to ghost_create, and it's also what you pass back to
// ghost_share_revoke to revoke the share.
type ShareOutput struct {
	URL          string     `json:"url"`
	ShareToken   string     `json:"share_token"`
	DatabaseID   string     `json:"database_id"`
	DatabaseName string     `json:"database_name"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

func (ShareOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ShareOutput](nil))
	shareOutputProperties(schema)
	return schema
}

// toShareOutput converts an API share into the MCP output shape, computing the
// status relative to now.
func toShareOutput(s api.DatabaseShare, baseURL string, now time.Time) (ShareOutput, error) {
	u, err := common.ShareURL(baseURL, s.ShareToken, s.DatabaseName)
	if err != nil {
		return ShareOutput{}, err
	}
	return ShareOutput{
		URL:          u,
		ShareToken:   s.ShareToken,
		DatabaseID:   s.DatabaseId,
		DatabaseName: s.DatabaseName,
		Status:       shareStatus(s, now),
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
		RevokedAt:    s.RevokedAt,
	}, nil
}

func newShareTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "ghost_share",
		Title: "Share Database",
		Description: `Share a database so a recipient can create their own database from a snapshot.

The share URL can be handed to anyone — they don't need access to this space. Pass the returned share_token to ghost_create (or ghost_create_dedicated) to create a new database from the shared snapshot.`,
		InputSchema:  ShareInput{}.Schema(),
		OutputSchema: ShareOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false,
			OpenWorldHint:   new(true),
			Title:           "Share Database",
		},
	}
}

func (s *Server) handleShare(ctx context.Context, req *mcp.CallToolRequest, input ShareInput) (*mcp.CallToolResult, ShareOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ShareOutput{}, err
	}

	if err := checkReadOnly(cfg); err != nil {
		return nil, ShareOutput{}, err
	}

	expiresAt, err := common.ParseExpires(input.Expires, time.Now())
	if err != nil {
		return nil, ShareOutput{}, err
	}

	// Fetch source database to check readiness (sharing snapshots the DB)
	getResp, err := client.GetDatabaseWithResponse(ctx, projectID, input.Ref)
	if err != nil {
		return nil, ShareOutput{}, fmt.Errorf("failed to get database details: %w", err)
	}
	if getResp.StatusCode() != http.StatusOK {
		return nil, ShareOutput{}, common.ExitWithErrorFromStatusCode(getResp.StatusCode(), getResp.JSONDefault)
	}
	if getResp.JSON200 == nil {
		return nil, ShareOutput{}, errors.New("empty response from API")
	}
	database := *getResp.JSON200

	if err := common.CheckReady(database); err != nil {
		return nil, ShareOutput{}, handleDatabaseError(err)
	}

	resp, err := client.ShareDatabaseWithResponse(ctx, projectID, database.Id, api.ShareDatabaseJSONRequestBody{
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, ShareOutput{}, fmt.Errorf("failed to create share: %w", err)
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, ShareOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON201 == nil {
		return nil, ShareOutput{}, errors.New("empty response from API")
	}

	output, err := toShareOutput(*resp.JSON201, cfg.ShareURL, time.Now())
	if err != nil {
		return nil, ShareOutput{}, err
	}
	return nil, output, nil
}

func shareStatus(s api.DatabaseShare, now time.Time) string {
	if s.RevokedAt != nil {
		return "revoked"
	}
	if s.ExpiresAt != nil && now.After(*s.ExpiresAt) {
		return "expired"
	}
	return "active"
}

// shareOutputProperties applies descriptions shared across share-related MCP outputs.
func shareOutputProperties(schema *jsonschema.Schema) {
	schema.Properties["url"].Description = "Landing-page URL a recipient opens to consume the share."
	schema.Properties["share_token"].Description = "Token a recipient passes to ghost_create as share_token to create a new database from the shared snapshot. Also pass this to ghost_share_revoke to revoke the share."
	schema.Properties["database_id"].Description = "Identifier of the shared database"
	schema.Properties["database_name"].Description = "Name of the shared database"
	schema.Properties["status"].Description = "Share status"
	schema.Properties["status"].Enum = []any{"active", "expired", "revoked"}
	schema.Properties["created_at"].Description = "Time the share was created"
	schema.Properties["expires_at"].Description = "Time after which the share expires (absent if it never expires)"
	schema.Properties["revoked_at"].Description = "Time the share was revoked (absent if still valid)"
}
