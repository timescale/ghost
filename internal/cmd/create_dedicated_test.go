package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestCreateDedicatedCmd(t *testing.T) {
	password := "testpass123"
	db := sampleDatabase(func(db *api.Database) {
		db.Password = &password
		db.Type = api.DatabaseTypeDedicated
	})

	defaultReq := api.CreateDatabaseRequest{
		Type: new(api.DatabaseTypeDedicated),
		Size: new(api.DatabaseSize("1x")),
	}

	namedReq := func(name string) api.CreateDatabaseRequest {
		return api.CreateDatabaseRequest{
			Name: new(name),
			Type: new(api.DatabaseTypeDedicated),
			Size: new(api.DatabaseSize("1x")),
		}
	}

	sizedReq := func(size string) api.CreateDatabaseRequest {
		return api.CreateDatabaseRequest{
			Type: new(api.DatabaseTypeDedicated),
			Size: new(api.DatabaseSize(size)),
		}
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"create", "dedicated", "--name", "mydb"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"create", "dedicated", "--name", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", namedReq("mydb")).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create database: connection refused",
		},
		{
			name: "API error",
			args: []string{"create", "dedicated", "--name", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", namedReq("mydb")).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal error"},
					}, nil)
			},
			wantErr: "internal error",
		},
		{
			name: "nil response body",
			args: []string{"create", "dedicated", "--name", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", namedReq("mydb")).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "auto-generated name",
			args: []string{"create", "dedicated"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				autoDb := sampleDatabase(func(db *api.Database) {
					db.Name = "ghost-12345"
					db.Password = &password
					db.Type = api.DatabaseTypeDedicated
				})
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", defaultReq).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &autoDb,
					}, nil)
			},
			wantStdout: "Created dedicated database 'ghost-12345' (size: 1x)\nID: abc1234567\nConnection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require\n",
		},
		{
			name: "text output with default size",
			args: []string{"create", "dedicated", "--name", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", namedReq("mydb")).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: `Created dedicated database 'mydb' (size: 1x)
ID: abc1234567
Connection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require
`,
		},
		{
			name: "text output with custom size",
			args: []string{"create", "dedicated", "--size", "4x"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", sizedReq("4x")).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: `Created dedicated database 'mydb' (size: 4x)
ID: abc1234567
Connection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require
`,
		},
		{
			name: "json output",
			args: []string{"create", "dedicated", "--name", "mydb", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", namedReq("mydb")).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: `{
  "name": "mydb",
  "id": "abc1234567",
  "size": "1x",
  "connection": "postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require"
}
`,
		},
		{
			name: "yaml output",
			args: []string{"create", "dedicated", "--name", "mydb", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", namedReq("mydb")).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: `connection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require
id: abc1234567
name: mydb
size: 1x
`,
		},
		{
			name: "with share token",
			args: []string{"create", "dedicated", "--from-share", "tok_xyz"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				req := api.CreateDatabaseRequest{
					Type:       new(api.DatabaseTypeDedicated),
					Size:       new(api.DatabaseSize("1x")),
					ShareToken: new("tok_xyz"),
				}
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", req).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			wantStdout: "Created dedicated database 'mydb' (size: 1x)\nID: abc1234567\nConnection: postgresql://tsdbadmin:testpass123@host.example.com:5432/tsdb?sslmode=require\n",
		},
	}

	runCmdTests(t, tests)
}
