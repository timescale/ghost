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
			name:    "name arg and --name flag conflict",
			args:    []string{"create", "mydb", "--name", "otherdb"},
			wantErr: "cannot specify both a name argument and the --name flag",
		},
		{
			name:    "not logged in",
			args:    []string{"create", "mydb"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"create", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create database: connection refused",
		},
		{
			name: "API error",
			args: []string{"create", "mydb"},
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
			// A standard (non-dedicated) create should never show the dedicated
			// payment-method guidance, even if the API returns the
			// NoPaymentMethod code — it falls through to the raw error instead.
			name: "no payment method code on standard create falls through to raw error",
			args: []string{"create", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSONDefault:  &api.Error{Message: "no valid payment method found", Code: new(api.ErrorCodeNoPaymentMethod)},
					}, nil)
			},
			wantErr: "no valid payment method found",
		},
		{
			name: "compute limit exceeded shows overages guidance",
			args: []string{"create", "mydb"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().CreateDatabaseWithResponse(validCtx, "test-project", api.CreateDatabaseRequest{Name: new("mydb")}).
					Return(&api.CreateDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSONDefault:  &api.Error{Message: "compute limit has been exceeded", Code: new(api.ErrorCodeComputeLimitExceeded)},
					}, nil)
			},
			wantErr: "this space has reached its compute limit, so you can't create a database\n\nRaise or remove the limit with 'ghost overages enable', or wait until your allowance\nresets next cycle",
		},
		{
			name: "nil response body",
			args: []string{"create", "mydb"},
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
			name: "name as positional arg",
			args: []string{"create", "mydb"},
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
			name: "name via deprecated --name flag",
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
			args: []string{"create", "mydb", "--json"},
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
			args: []string{"create", "mydb", "--yaml"},
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
