package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestForkDedicatedCmd(t *testing.T) {
	password := "forkpass"

	sourceDb := sampleDatabase()

	forkedDb := sampleDatabase(func(db *api.Database) {
		db.Id = "forked1234"
		db.Name = "mydb-fork"
		db.Host = "fork.example.com"
		db.Password = &password
		db.Type = api.DatabaseTypeDedicated
	})

	setupGetSource := func(m *mock.MockClientWithResponsesInterface) {
		db := sourceDb
		m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "abc1234567").
			Return(&api.GetDatabaseResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &db,
			}, nil)
	}

	defaultReq := api.ForkDatabaseRequest{
		Type: new(api.DatabaseTypeDedicated),
		Size: new(api.DatabaseSize("1x")),
	}

	setupForkSuccess := func(name *string) func(m *mock.MockClientWithResponsesInterface) {
		return func(m *mock.MockClientWithResponsesInterface) {
			db := forkedDb
			req := api.ForkDatabaseRequest{
				Name: name,
				Type: new(api.DatabaseTypeDedicated),
				Size: new(api.DatabaseSize("1x")),
			}
			m.EXPECT().ForkDatabaseWithResponse(validCtx, "test-project", "abc1234567", req).
				Return(&api.ForkDatabaseResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &db,
				}, nil)
		}
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"fork", "dedicated", "abc1234567"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error on get source",
			args: []string{"fork", "dedicated", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().GetDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get source database: connection refused",
		},
		{
			name: "API error on get source",
			args: []string{"fork", "dedicated", "abc1234567"},
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
			name: "nil response body on get source",
			args: []string{"fork", "dedicated", "abc1234567"},
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
			name: "source database paused",
			args: []string{"fork", "dedicated", "abc1234567"},
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
			name: "source database not ready",
			args: []string{"fork", "dedicated", "abc1234567"},
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
			name: "network error on fork",
			args: []string{"fork", "dedicated", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGetSource(m)
				m.EXPECT().ForkDatabaseWithResponse(validCtx, "test-project", "abc1234567", defaultReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fork database: connection refused",
		},
		{
			name: "API error on fork",
			args: []string{"fork", "dedicated", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGetSource(m)
				m.EXPECT().ForkDatabaseWithResponse(validCtx, "test-project", "abc1234567", defaultReq).
					Return(&api.ForkDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal server error"},
					}, nil)
			},
			wantErr: "internal server error",
		},
		{
			name: "nil response body on fork",
			args: []string{"fork", "dedicated", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGetSource(m)
				m.EXPECT().ForkDatabaseWithResponse(validCtx, "test-project", "abc1234567", defaultReq).
					Return(&api.ForkDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "text output with default size",
			args: []string{"fork", "dedicated", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGetSource(m)
				setupForkSuccess(nil)(m)
			},
			wantStdout: "Forked 'mydb' → dedicated 'mydb-fork' (size: 1x)\nID: forked1234\nConnection: postgresql://tsdbadmin:forkpass@fork.example.com:5432/tsdb?sslmode=require\n",
		},
		{
			name: "text output with custom name",
			args: []string{"fork", "dedicated", "abc1234567", "--name", "custom-fork"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGetSource(m)
				setupForkSuccess(new("custom-fork"))(m)
			},
			wantStdout: "Forked 'mydb' → dedicated 'mydb-fork' (size: 1x)\nID: forked1234\nConnection: postgresql://tsdbadmin:forkpass@fork.example.com:5432/tsdb?sslmode=require\n",
		},
		{
			name: "text output with custom size",
			args: []string{"fork", "dedicated", "abc1234567", "--size", "4x"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGetSource(m)
				db := forkedDb
				m.EXPECT().ForkDatabaseWithResponse(validCtx, "test-project", "abc1234567", api.ForkDatabaseRequest{
					Type: new(api.DatabaseTypeDedicated),
					Size: new(api.DatabaseSize("4x")),
				}).Return(&api.ForkDatabaseResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &db,
				}, nil)
			},
			wantStdout: "Forked 'mydb' → dedicated 'mydb-fork' (size: 4x)\nID: forked1234\nConnection: postgresql://tsdbadmin:forkpass@fork.example.com:5432/tsdb?sslmode=require\n",
		},
		{
			name: "json output",
			args: []string{"fork", "dedicated", "abc1234567", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGetSource(m)
				setupForkSuccess(nil)(m)
			},
			wantStdout: `{
  "source_name": "mydb",
  "name": "mydb-fork",
  "id": "forked1234",
  "size": "1x",
  "connection": "postgresql://tsdbadmin:forkpass@fork.example.com:5432/tsdb?sslmode=require"
}
`,
		},
		{
			name: "yaml output",
			args: []string{"fork", "dedicated", "abc1234567", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				setupGetSource(m)
				setupForkSuccess(nil)(m)
			},
			wantStdout: `connection: postgresql://tsdbadmin:forkpass@fork.example.com:5432/tsdb?sslmode=require
id: forked1234
name: mydb-fork
size: 1x
source_name: mydb
`,
		},
	}

	runCmdTests(t, tests)
}
