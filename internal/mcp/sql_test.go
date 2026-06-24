package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
)

func TestCapResultSetRows(t *testing.T) {
	rows := func(n int) [][]string {
		out := make([][]string, n)
		for i := range out {
			out[i] = []string{"v"}
		}
		return out
	}

	tests := []struct {
		name     string
		sets     []common.ResultSet
		limit    int
		wantRows []int
	}{
		{
			name:     "no sets",
			sets:     nil,
			limit:    50,
			wantRows: nil,
		},
		{
			name:     "under limit is untouched",
			sets:     []common.ResultSet{{Rows: rows(3)}},
			limit:    50,
			wantRows: []int{3},
		},
		{
			name:     "exactly at limit is untouched",
			sets:     []common.ResultSet{{Rows: rows(50)}},
			limit:    50,
			wantRows: []int{50},
		},
		{
			name:     "over limit is truncated",
			sets:     []common.ResultSet{{Rows: rows(120)}},
			limit:    50,
			wantRows: []int{50},
		},
		{
			name: "each set capped independently",
			sets: []common.ResultSet{
				{Rows: rows(2)},
				{Rows: rows(75)},
			},
			limit:    50,
			wantRows: []int{2, 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capResultSetRows(tt.sets, tt.limit)
			if len(tt.sets) != len(tt.wantRows) {
				t.Fatalf("set count = %d, want %d", len(tt.sets), len(tt.wantRows))
			}
			for i := range tt.sets {
				if got := len(tt.sets[i].Rows); got != tt.wantRows[i] {
					t.Errorf("set %d rows = %d, want %d", i, got, tt.wantRows[i])
				}
			}
		})
	}
}

// TestCapResultSetRowsPreservesRowsAffected ensures truncating returned rows
// does not overwrite RowsAffected, which reflects the Postgres command tag
// (e.g. rows touched by an UPDATE/DELETE), not the count of rows returned.
func TestCapResultSetRowsPreservesRowsAffected(t *testing.T) {
	sets := []common.ResultSet{{
		Rows:         make([][]string, 100),
		RowsAffected: 100,
	}}
	capResultSetRows(sets, 10)

	if got := len(sets[0].Rows); got != 10 {
		t.Errorf("rows = %d, want 10", got)
	}
	if sets[0].RowsAffected != 100 {
		t.Errorf("RowsAffected = %d, want 100 (must not be overwritten by truncation)", sets[0].RowsAffected)
	}
}

// newTestApp returns a *common.App loaded against an isolated temp config dir,
// with the API client provided by clientFactory (which may be nil to simulate
// the user not being logged in — GetAll/GetClient then surface clientErr).
func newTestApp(t *testing.T, clientErr error) *common.App {
	t.Helper()

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("config-dir", config.DefaultConfigDir, "config directory")
	flags.Bool("analytics", true, "")
	flags.Bool("color", true, "")
	flags.Bool("version-check", true, "")
	if err := flags.Set("config-dir", t.TempDir()); err != nil {
		t.Fatalf("setting config-dir: %v", err)
	}

	app := &common.App{}
	app.SetFlags(flags)
	app.SetClientFactory(func(ctx context.Context, cfg *config.Config) (api.ClientWithResponsesInterface, string, error) {
		if clientErr != nil {
			return nil, "", clientErr
		}
		return mock.NewMockClientWithResponsesInterface(nil), "test-project", nil
	})
	if _, _, _, err := app.Load(context.Background()); err != nil {
		t.Fatalf("loading app: %v", err)
	}
	return app
}

func TestHandleSQLQueryFileValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   SQLInput
		wantErr string
	}{
		{
			name:    "neither query nor file",
			input:   SQLInput{Ref: "db"},
			wantErr: "exactly one of 'query' or 'file' must be provided",
		},
		{
			name:    "both query and file",
			input:   SQLInput{Ref: "db", Query: "SELECT 1", File: "q.sql"},
			wantErr: "exactly one of 'query' or 'file' must be provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{app: newTestApp(t, nil)}
			_, _, err := s.handleSQL(context.Background(), nil, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestHandleSQLVisualizeRequiresBrowser verifies that requesting visualization
// without a browser-backed server (i.e. not local/stdio mode, where s.browser
// is nil) fails fast with a clear error rather than attempting to connect.
func TestHandleSQLVisualizeRequiresBrowser(t *testing.T) {
	s := &Server{app: newTestApp(t, nil)} // browser is nil (remote mode)

	_, _, err := s.handleSQL(context.Background(), nil, SQLInput{
		Ref:       "db",
		Query:     "SELECT 1",
		Visualize: "table",
	})
	if err == nil || !strings.Contains(err.Error(), "visualization is only available when running the MCP server locally") {
		t.Fatalf("err = %v, want visualization-not-available error", err)
	}
}
