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
