package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestCreateCmd(t *testing.T) {
	password := "testpass123"
	db := sampleDatabase(func(db *api.Database) {
		db.Password = &password
	})

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"create", "--name", "mydb"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"create", "--name", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create database: connection refused",
		},
		{
			name: "API error",
			args: []string{"create", "--name", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal error"},
					}, nil)
			},
			wantErr: "internal error",
		},
		{
			name: "nil response body",
			args: []string{"create", "--name", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "auto-generated name",
			args: []string{"create"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				autoDb := sampleDatabase(func(db *api.Database) {
					db.Name = "ghost-12345"
					db.Password = &password
				})
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &autoDb,
					}, nil)
			},
			wantStdout: "Created database 'ghost-12345'\nID: abc1234567\nConnection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require\n",
		},
		{
			name: "text output",
			args: []string{"create", "--name", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: `Created database 'mydb'
ID: abc1234567
Connection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require
`,
		},
		{
			name: "json output",
			args: []string{"create", "--name", "mydb", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: `{
  "name": "mydb",
  "id": "abc1234567",
  "connection": "postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require"
}
`,
		},
		{
			name: "yaml output",
			args: []string{"create", "--name", "mydb", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: `connection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require
id: abc1234567
name: mydb
`,
		},
		{
			name: "with share token",
			args: []string{"create", "--from-share", "tok_xyz"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{ShareToken: new("tok_xyz")}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: "Created database 'mydb'\nID: abc1234567\nConnection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require\n",
		},
	}

	runCmdTests(t, tests)
}
