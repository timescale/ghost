package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestShareCmd(t *testing.T) {
	createdAt := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	share := api.DatabaseShare{
		Id:           "shr_abc",
		ShareToken:   "tok_xyz",
		DatabaseId:   "abc1234567",
		DatabaseName: "mydb",
		CreatedAt:    createdAt,
	}

	setupGet := func(m *mock.MockClientWithResponsesInterface) {
		db := sampleDatabase()
		m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "abc1234567").
			Return(&api.GetDatabaseResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &db,
			}, nil)
	}

	setupShareSuccess := func(expiresAt *time.Time, withExpiry bool) func(m *mock.MockClientWithResponsesInterface) {
		return func(m *mock.MockClientWithResponsesInterface) {
			s := share
			if withExpiry {
				s.ExpiresAt = expiresAt
			}
			m.EXPECT().ShareDatabaseWithResponse(validCtx, "test-project", "abc1234567", api.ShareDatabaseJSONRequestBody{ExpiresAt: expiresAt}).
				Return(&api.ShareDatabaseResponse{
					HTTPResponse: httpResponse(http.StatusCreated),
					JSON201:      &s,
				}, nil)
		}
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"share", "abc1234567"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error on get",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get database details: connection refused",
		},
		{
			name: "API error on get",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(&api.GetDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSONDefault:  &api.Error{Message: "database not found"},
					}, nil)
			},
			wantErr: "database not found",
		},
		{
			name: "nil response body on get",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(&api.GetDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "database paused",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				db := sampleDatabase(func(db *api.Database) {
					db.Status = api.DatabaseStatusPaused
				})
				m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(&api.GetDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &db,
					}, nil)
			},
			wantErr: "database is currently paused — resume it with 'ghost resume abc1234567'",
		},
		{
			name: "database not ready",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				db := sampleDatabase(func(db *api.Database) {
					db.Status = api.DatabaseStatusConfiguring
				})
				m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(&api.GetDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &db,
					}, nil)
			},
			wantErr: "database is not yet ready — check status with 'ghost list' and try again",
		},
		{
			name: "network error on share",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGet(m)
				m.EXPECT().ShareDatabaseWithResponse(validCtx, "test-project", "abc1234567", api.ShareDatabaseJSONRequestBody{}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create share: connection refused",
		},
		{
			name: "API error on share",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGet(m)
				m.EXPECT().ShareDatabaseWithResponse(validCtx, "test-project", "abc1234567", api.ShareDatabaseJSONRequestBody{}).
					Return(&api.ShareDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal server error"},
					}, nil)
			},
			wantErr: "internal server error",
		},
		{
			name: "nil response body on share",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGet(m)
				m.EXPECT().ShareDatabaseWithResponse(validCtx, "test-project", "abc1234567", api.ShareDatabaseJSONRequestBody{}).
					Return(&api.ShareDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusCreated),
						JSON201:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "invalid --expires-at",
			args: []string{"share", "abc1234567", "--expires-at", "not-a-date"},
			// no mock: error is caught before any API call
			wantErr: `invalid --expires-at (expected RFC3339 like 2026-05-01T00:00:00Z): parsing time "not-a-date" as "2006-01-02T15:04:05Z07:00": cannot parse "not-a-date" as "2006"`,
		},
		{
			name: "text output no expiry",
			args: []string{"share", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGet(m)
				setupShareSuccess(nil, false)(m)
			},
			wantStdout: "Shared 'mydb'\nURL: https://ghost.build/share/tok_xyz\nToken: tok_xyz\nExpires: never\n",
		},
		{
			name: "text output with --expires-at",
			args: []string{"share", "abc1234567", "--expires-at", "2026-05-01T00:00:00Z"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGet(m)
				setupShareSuccess(&expiresAt, true)(m)
			},
			wantStdout: "Shared 'mydb'\nURL: https://ghost.build/share/tok_xyz\nToken: tok_xyz\nExpires: 2026-05-01T00:00:00Z\n",
		},
		{
			name: "json output",
			args: []string{"share", "abc1234567", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGet(m)
				setupShareSuccess(nil, false)(m)
			},
			wantStdout: `{
  "url": "https://ghost.build/share/tok_xyz",
  "share_token": "tok_xyz",
  "database_id": "abc1234567",
  "database_name": "mydb",
  "status": "active",
  "created_at": "2026-04-23T12:00:00Z"
}
`,
		},
		{
			name: "yaml output",
			args: []string{"share", "abc1234567", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGet(m)
				setupShareSuccess(nil, false)(m)
			},
			wantStdout: `created_at: "2026-04-23T12:00:00Z"
database_id: abc1234567
database_name: mydb
share_token: tok_xyz
status: active
url: https://ghost.build/share/tok_xyz
`,
		},
	}

	runCmdTests(t, tests)
}
