package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
)

// Share represents a database share in CLI output. The share_token (also
// embedded in the URL) is the only identifier exposed — it's what a recipient
// passes to 'ghost create', and it's also what you pass back to
// 'ghost share revoke' to revoke the share.
type Share struct {
	URL          string     `json:"url"`
	ShareToken   string     `json:"share_token"`
	DatabaseID   string     `json:"database_id"`
	DatabaseName string     `json:"database_name"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

func buildShareCmd(app *common.App) *cobra.Command {
	cmd := buildShareCreateCmd(app)
	cmd.AddCommand(buildShareListCmd(app))
	cmd.AddCommand(buildShareRevokeCmd(app))
	return cmd
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

// toShare converts an API share into the CLI output shape, computing the
// status relative to now.
func toShare(s api.DatabaseShare, baseURL string, now time.Time) Share {
	return Share{
		URL:          common.ShareURL(baseURL, s.ShareToken),
		ShareToken:   s.ShareToken,
		DatabaseID:   s.DatabaseId,
		DatabaseName: s.DatabaseName,
		Status:       shareStatus(s, now),
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
		RevokedAt:    s.RevokedAt,
	}
}
