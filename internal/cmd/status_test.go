package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestStatusCmd(t *testing.T) {
	successSetup := func(m *mock.MockClientWithResponsesInterface) {
		m.EXPECT().SpaceStatusWithResponse(validCtx, "test-project").
			Return(&api.SpaceStatusResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.SpaceStatus{
					ComputeMinutes:      120,
					ComputeLimitMinutes: 600,
					StorageMib:          512,
					StorageLimitMib:     1048576,
				},
			}, nil)
		databases := []api.DatabaseWithUsage{
			sampleDatabaseWithUsage(),
			sampleDatabaseWithUsage(func(db *api.DatabaseWithUsage) {
				db.Id = "def4567890"
				db.Name = "otherdb"
				db.Status = api.DatabaseStatusPaused
			}),
		}
		m.EXPECT().ListDatabasesWithResponse(validCtx, "test-project").
			Return(&api.ListDatabasesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &databases,
			}, nil)
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"status"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error on space status",
			args: []string{"status"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().SpaceStatusWithResponse(validCtx, "test-project").
					Return(nil, errors.New("connection refused"))
				databases := []api.DatabaseWithUsage{}
				m.EXPECT().ListDatabasesWithResponse(validCtx, "test-project").
					Return(&api.ListDatabasesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &databases,
					}, nil).AnyTimes()
			},
			wantErr: "failed to get space status: connection refused",
		},
		{
			name: "API error on space status",
			args: []string{"status"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().SpaceStatusWithResponse(validCtx, "test-project").
					Return(&api.SpaceStatusResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal error"},
					}, nil)
				databases := []api.DatabaseWithUsage{}
				m.EXPECT().ListDatabasesWithResponse(validCtx, "test-project").
					Return(&api.ListDatabasesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &databases,
					}, nil).AnyTimes()
			},
			wantErr: "internal error",
		},
		{
			name: "nil space status response body",
			args: []string{"status"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().SpaceStatusWithResponse(validCtx, "test-project").
					Return(&api.SpaceStatusResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
				databases := []api.DatabaseWithUsage{}
				m.EXPECT().ListDatabasesWithResponse(validCtx, "test-project").
					Return(&api.ListDatabasesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &databases,
					}, nil).AnyTimes()
			},
			wantErr: "empty response from API",
		},
		{
			name: "nil list databases response body",
			args: []string{"status"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().SpaceStatusWithResponse(validCtx, "test-project").
					Return(&api.SpaceStatusResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200: &api.SpaceStatus{
							ComputeMinutes:      120,
							ComputeLimitMinutes: 600,
							StorageMib:          512,
							StorageLimitMib:     1048576,
						},
					}, nil).AnyTimes()
				m.EXPECT().ListDatabasesWithResponse(validCtx, "test-project").
					Return(&api.ListDatabasesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:  "text output",
			args:  []string{"status"},
			setup: successSetup,
			wantStdout: `Space: test-project
Compute: 2/10 hours (20%)
Storage: 512MiB/1TiB (0%)
Databases: 2 (1 running, 1 paused)
`,
		},
		{
			name:  "json output",
			args:  []string{"status", "--json"},
			setup: successSetup,
			wantStdout: `{
  "compute_minutes": 120,
  "compute_limit_minutes": 600,
  "storage_mib": 512,
  "storage_limit_mib": 1048576,
  "databases": {
    "running": 1,
    "paused": 1
  },
  "space_id": "test-project"
}
`,
		},
		{
			name:  "yaml output",
			args:  []string{"status", "--yaml"},
			setup: successSetup,
			wantStdout: `compute_limit_minutes: 600
compute_minutes: 120
databases:
  paused: 1
  running: 1
space_id: test-project
storage_limit_mib: 1.048576e+06
storage_mib: 512
`,
		},
		{
			name:  "usage alias",
			args:  []string{"usage"},
			setup: successSetup,
			wantStdout: `Space: test-project
Compute: 2/10 hours (20%)
Storage: 512MiB/1TiB (0%)
Databases: 2 (1 running, 1 paused)
`,
		},
		{
			name: "text output with cost",
			args: []string{"status"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().SpaceStatusWithResponse(validCtx, "test-project").
					Return(&api.SpaceStatusResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200: &api.SpaceStatus{
							ComputeMinutes:      120,
							ComputeLimitMinutes: 600,
							StorageMib:          512,
							StorageLimitMib:     1048576,
							CostToDate:          new(12.34),
							EstimatedTotalCost:  new(27.50),
						},
					}, nil)
				databases := []api.DatabaseWithUsage{sampleDatabaseWithUsage()}
				m.EXPECT().ListDatabasesWithResponse(validCtx, "test-project").
					Return(&api.ListDatabasesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &databases,
					}, nil)
			},
			wantStdout: `Space: test-project
Compute: 2/10 hours (20%)
Storage: 512MiB/1TiB (0%)
Databases: 1 (1 running)
Cost: $12.34 so far this cycle ($27.50 estimated total)
`,
		},
		{
			name: "text output with zero cost is omitted",
			args: []string{"status"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().SpaceStatusWithResponse(validCtx, "test-project").
					Return(&api.SpaceStatusResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200: &api.SpaceStatus{
							ComputeMinutes:      120,
							ComputeLimitMinutes: 600,
							StorageMib:          512,
							StorageLimitMib:     1048576,
							CostToDate:          new(0.0),
							EstimatedTotalCost:  new(0.0),
						},
					}, nil)
				databases := []api.DatabaseWithUsage{sampleDatabaseWithUsage()}
				m.EXPECT().ListDatabasesWithResponse(validCtx, "test-project").
					Return(&api.ListDatabasesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &databases,
					}, nil)
			},
			wantStdout: `Space: test-project
Compute: 2/10 hours (20%)
Storage: 512MiB/1TiB (0%)
Databases: 1 (1 running)
`,
		},
	}

	runCmdTests(t, tests)
}
