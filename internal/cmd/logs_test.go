package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

// logsParamsMatcher returns a gomock.Matcher that checks DatabaseLogsParams
// fields. The cursor argument must match exactly (nil for the first call,
// pointing to the expected value for subsequent calls). If until is non-zero,
// Until must match exactly; otherwise Until is only checked for non-nil
// (since FetchLogs defaults to time.Now()).
func logsParamsMatcher(cursor *string, until time.Time) gomock.Matcher {
	return gomock.Cond(func(x any) bool {
		params, ok := x.(*api.DatabaseLogsParams)
		if !ok || params == nil {
			return false
		}
		switch {
		case cursor == nil && params.Cursor != nil:
			return false
		case cursor != nil && (params.Cursor == nil || *params.Cursor != *cursor):
			return false
		}
		if !until.IsZero() {
			return params.Until != nil && params.Until.Equal(until)
		}
		return params.Until != nil
	})
}

// logEntry is a test helper that builds a LogEntry with the given timestamp
// and message, using the default LOG severity.
func logEntry(timestamp time.Time, message string) api.LogEntry {
	return api.LogEntry{
		Timestamp: timestamp,
		Message:   message,
		Severity:  "LOG",
	}
}

func TestLogsCmd(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"logs", "abc1234567"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
		{
			name: "network error",
			args: []string{"logs", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch logs: connection refused",
		},
		{
			name: "API error",
			args: []string{"logs", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
					Return(&api.DatabaseLogsResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSONDefault:  &api.Error{Message: "internal error"},
					}, nil)
			},
			wantErr: "internal error",
		},
		{
			name: "nil response body",
			args: []string{"logs", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
					Return(&api.DatabaseLogsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "text output",
			args: []string{"logs", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
					Return(&api.DatabaseLogsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200: &api.LogsResponse{
							Entries: []api.LogEntry{
								logEntry(t2, "msg2"),
								logEntry(t1, "msg1"),
							},
						},
					}, nil)
			},
			wantStdout: "2024-01-01 00:00:00 UTC msg1\n2024-01-02 00:00:00 UTC msg2\n",
		},
		{
			name: "json output",
			args: []string{"logs", "abc1234567", "--json"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
					Return(&api.DatabaseLogsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200: &api.LogsResponse{
							Entries: []api.LogEntry{
								logEntry(t2, "msg2"),
								logEntry(t1, "msg1"),
							},
						},
					}, nil)
			},
			wantStdout: `[
  "2024-01-01 00:00:00 UTC msg1",
  "2024-01-02 00:00:00 UTC msg2"
]
`,
		},
		{
			name: "yaml output",
			args: []string{"logs", "abc1234567", "--yaml"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
					Return(&api.DatabaseLogsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200: &api.LogsResponse{
							Entries: []api.LogEntry{
								logEntry(t2, "msg2"),
								logEntry(t1, "msg1"),
							},
						},
					}, nil)
			},
			wantStdout: `- 2024-01-01 00:00:00 UTC msg1
- 2024-01-02 00:00:00 UTC msg2
`,
		},
		{
			name: "empty logs",
			args: []string{"logs", "abc1234567"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
					Return(&api.DatabaseLogsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &api.LogsResponse{Entries: []api.LogEntry{}},
					}, nil)
			},
			wantStdout: "",
		},
		{
			name:    "negative tail",
			args:    []string{"logs", "abc1234567", "--tail", "-1"},
			wantErr: "--tail must be at least 1, got -1",
		},
		{
			name:    "zero tail",
			args:    []string{"logs", "abc1234567", "--tail", "0"},
			wantErr: "--tail must be at least 1, got 0",
		},
		{
			name: "tail flag",
			args: []string{"logs", "abc1234567", "--tail", "1"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
					Return(&api.DatabaseLogsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200: &api.LogsResponse{
							Entries: []api.LogEntry{
								logEntry(t2, "msg2"),
								logEntry(t1, "msg1"),
							},
						},
					}, nil)
			},
			wantStdout: "2024-01-02 00:00:00 UTC msg2\n",
		},
		{
			name: "cursor pagination",
			args: []string{"logs", "abc1234567", "--tail", "3"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				cursor := "page-2-cursor"
				gomock.InOrder(
					m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, time.Time{})).
						Return(&api.DatabaseLogsResponse{
							HTTPResponse: httpResponse(http.StatusOK),
							JSON200: &api.LogsResponse{
								Entries: []api.LogEntry{
									logEntry(t2, "msg3"),
									logEntry(t2, "msg2"),
								},
								LastCursor: &cursor,
							},
						}, nil),
					m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(&cursor, time.Time{})).
						Return(&api.DatabaseLogsResponse{
							HTTPResponse: httpResponse(http.StatusOK),
							JSON200: &api.LogsResponse{
								Entries: []api.LogEntry{logEntry(t1, "msg1")},
							},
						}, nil),
				)
			},
			wantStdout: "2024-01-01 00:00:00 UTC msg1\n2024-01-02 00:00:00 UTC msg2\n2024-01-02 00:00:00 UTC msg3\n",
		},
		{
			name: "until flag",
			args: []string{"logs", "abc1234567", "--until", "2024-06-15T10:00:00Z"},
			setup: func(m *mock.MockClientWithResponsesInterface) {
				until := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
				m.EXPECT().DatabaseLogsWithResponse(validCtx, "test-project", "abc1234567", logsParamsMatcher(nil, until)).
					Return(&api.DatabaseLogsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200: &api.LogsResponse{
							Entries: []api.LogEntry{logEntry(t1, "log before cutoff")},
						},
					}, nil)
			},
			wantStdout: "2024-01-01 00:00:00 UTC log before cutoff\n",
		},
	}

	runCmdTests(t, tests)
}
