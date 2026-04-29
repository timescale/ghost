package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestShareListCmd(t *testing.T) {
	createdAt := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	futureExpiry := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	pastExpiry := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	revokedAt := time.Date(2026, 4, 24, 9, 0, 0, 0, time.UTC)

	active := api.DatabaseShare{
		ShareToken:   "tok_a",
		DatabaseId:   "abc1234567",
		DatabaseName: "mydb",
		CreatedAt:    createdAt,
		ExpiresAt:    &futureExpiry,
	}
	expired := api.DatabaseShare{
		ShareToken:   "tok_e",
		DatabaseId:   "def7654321",
		DatabaseName: "otherdb",
		CreatedAt:    createdAt,
		ExpiresAt:    &pastExpiry,
	}
	revoked := api.DatabaseShare{
		ShareToken:   "tok_r",
		DatabaseId:   "ghi9999999",
		DatabaseName: "thirddb",
		CreatedAt:    createdAt,
		RevokedAt:    &revokedAt,
	}

	shares := []api.DatabaseShare{active, expired, revoked}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"share", "list"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"share", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListSharesWithResponse(validCtx, "test-project").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list shares: connection refused",
		},
		{
			name: "API error",
			args: []string{"share", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListSharesWithResponse(validCtx, "test-project").
					Return(&api.ListSharesResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal server error"},
					}, nil)
			},
			wantErr: "internal server error",
		},
		{
			name: "nil response body",
			args: []string{"share", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ListSharesWithResponse(validCtx, "test-project").
					Return(&api.ListSharesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "empty list",
			args: []string{"share", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				empty := []api.DatabaseShare{}
				m.EXPECT().ListSharesWithResponse(validCtx, "test-project").
					Return(&api.ListSharesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &empty,
					}, nil)
			},
			wantStdout: "No shares found.\nRun 'ghost share <database>' to create a share.\n",
		},
		{
			name: "text output",
			args: []string{"share", "list"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				list := shares
				m.EXPECT().ListSharesWithResponse(validCtx, "test-project").
					Return(&api.ListSharesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &list,
					}, nil)
			},
			wantStdout: "DATABASE  STATUS   CREATED               EXPIRES               TOKEN  \nmydb      active   2026-04-23T12:00:00Z  2099-01-01T00:00:00Z  tok_a  \notherdb   expired  2026-04-23T12:00:00Z  2020-01-01T00:00:00Z  tok_e  \nthirddb   revoked  2026-04-23T12:00:00Z  never                 tok_r  \n",
		},
		{
			name: "json output",
			args: []string{"share", "list", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				list := []api.DatabaseShare{active}
				m.EXPECT().ListSharesWithResponse(validCtx, "test-project").
					Return(&api.ListSharesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &list,
					}, nil)
			},
			wantStdout: `[
  {
    "url": "https://ghost.build/share/tok_a?name=mydb",
    "share_token": "tok_a",
    "database_id": "abc1234567",
    "database_name": "mydb",
    "status": "active",
    "created_at": "2026-04-23T12:00:00Z",
    "expires_at": "2099-01-01T00:00:00Z"
  }
]
`,
		},
		{
			name: "yaml output",
			args: []string{"share", "list", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				list := []api.DatabaseShare{active}
				m.EXPECT().ListSharesWithResponse(validCtx, "test-project").
					Return(&api.ListSharesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &list,
					}, nil)
			},
			wantStdout: `- created_at: "2026-04-23T12:00:00Z"
  database_id: abc1234567
  database_name: mydb
  expires_at: "2099-01-01T00:00:00Z"
  share_token: tok_a
  status: active
  url: https://ghost.build/share/tok_a?name=mydb
`,
		},
		{
			name: "ls alias",
			args: []string{"share", "ls"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				list := []api.DatabaseShare{active}
				m.EXPECT().ListSharesWithResponse(validCtx, "test-project").
					Return(&api.ListSharesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &list,
					}, nil)
			},
			wantStdout: "DATABASE  STATUS  CREATED               EXPIRES               TOKEN  \nmydb      active  2026-04-23T12:00:00Z  2099-01-01T00:00:00Z  tok_a  \n",
		},
	}

	runCmdTests(t, tests)
}
