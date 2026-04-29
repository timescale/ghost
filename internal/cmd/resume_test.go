package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestResumeCmd(t *testing.T) {
	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"resume", "abc1234567"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"resume", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ResumeDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to resume database: connection refused",
		},
		{
			name: "API error",
			args: []string{"resume", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ResumeDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(&api.ResumeDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSONDefault:  &api.Error{Message: "database not found"},
					}, nil)
			},
			wantErr: "database not found",
		},
		{
			name: "nil response body",
			args: []string{"resume", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().ResumeDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(&api.ResumeDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "text output",
			args: []string{"resume", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				db := sampleDatabase()
				m.EXPECT().ResumeDatabaseWithResponse(validCtx, "test-project", "abc1234567").
					Return(&api.ResumeDatabaseResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &db,
					}, nil)
			},
			opts:       []runOption{withEnv("HOME", t.TempDir())},
			wantStderr: "Warning: failed to get password: password not found\n",
			wantStdout: `Resuming 'mydb' (abc1234567)...
Connection: postgresql://tsdbadmin@host.example.com:5432/tsdb?sslmode=require
`,
		},
	}

	runCmdTests(t, tests)
}
