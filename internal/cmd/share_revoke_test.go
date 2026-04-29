package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestShareRevokeCmd(t *testing.T) {
	createdAt := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	revokedAt := time.Date(2026, 4, 24, 9, 0, 0, 0, time.UTC)

	revoked := api.DatabaseShare{
		ShareToken:   "tok_xyz",
		DatabaseId:   "abc1234567",
		DatabaseName: "mydb",
		CreatedAt:    createdAt,
		RevokedAt:    &revokedAt,
	}

	setupRevokeSuccess := func(m *mock.MockClientWithResponsesInterface) {
		r := revoked
		m.EXPECT().RevokeShareWithResponse(validCtx, "test-project", "tok_xyz").
			Return(&api.RevokeShareResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &r,
			}, nil)
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"share", "revoke", "tok_xyz"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error on revoke",
			args: []string{"share", "revoke", "tok_xyz"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().RevokeShareWithResponse(validCtx, "test-project", "tok_xyz").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to revoke share: connection refused",
		},
		{
			name: "token not found",
			args: []string{"share", "revoke", "tok_unknown"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().RevokeShareWithResponse(validCtx, "test-project", "tok_unknown").
					Return(&api.RevokeShareResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSONDefault:  &api.Error{Message: "share not found"},
					}, nil)
			},
			wantErr: "share not found",
		},
		{
			name: "API error on revoke",
			args: []string{"share", "revoke", "tok_xyz"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().RevokeShareWithResponse(validCtx, "test-project", "tok_xyz").
					Return(&api.RevokeShareResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal server error"},
					}, nil)
			},
			wantErr: "internal server error",
		},
		{
			name: "nil response body on revoke",
			args: []string{"share", "revoke", "tok_xyz"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().RevokeShareWithResponse(validCtx, "test-project", "tok_xyz").
					Return(&api.RevokeShareResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "text output",
			args:       []string{"share", "revoke", "tok_xyz"},
			setup:      setupRevokeSuccess,
			wantStdout: "Revoked share for 'mydb'\n",
		},
		{
			name:  "json output",
			args:  []string{"share", "revoke", "tok_xyz", "--json"},
			setup: setupRevokeSuccess,
			wantStdout: `{
  "url": "https://ghost.build/share/tok_xyz?name=mydb",
  "share_token": "tok_xyz",
  "database_id": "abc1234567",
  "database_name": "mydb",
  "status": "revoked",
  "created_at": "2026-04-23T12:00:00Z",
  "revoked_at": "2026-04-24T09:00:00Z"
}
`,
		},
		{
			name:  "yaml output",
			args:  []string{"share", "revoke", "tok_xyz", "--yaml"},
			setup: setupRevokeSuccess,
			wantStdout: `created_at: "2026-04-23T12:00:00Z"
database_id: abc1234567
database_name: mydb
revoked_at: "2026-04-24T09:00:00Z"
share_token: tok_xyz
status: revoked
url: https://ghost.build/share/tok_xyz?name=mydb
`,
		},
	}

	runCmdTests(t, tests)
}
