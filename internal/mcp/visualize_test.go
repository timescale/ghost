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

// TestHandleVisualizeRequiresBrowser verifies that ghost_visualize fails fast
// with a clear error when there's no browser-backed server (i.e. not
// local/stdio mode, where s.browser is nil) rather than attempting to connect.
func TestHandleVisualizeRequiresBrowser(t *testing.T) {
	s := &Server{app: newTestApp(t, nil)} // browser is nil (remote mode)

	_, _, err := s.handleVisualize(context.Background(), nil, VisualizeInput{
		Ref: "db",
		SQL: "SELECT 1",
	})
	if err == nil || !strings.Contains(err.Error(), "visualization is only available when running the MCP server locally") {
		t.Fatalf("err = %v, want visualization-not-available error", err)
	}
}

// TestHandleVisualizeRequiresSQLOrChartConfig verifies that providing neither
// sql nor chart_config is rejected with a clear error. The browser-nil check
// runs first, so this uses a non-nil (but unused) browser controller.
func TestHandleVisualizeRequiresSQLOrChartConfig(t *testing.T) {
	app := newTestApp(t, nil)
	s := &Server{app: app, browser: newBrowserController(app, nil)}

	_, _, err := s.handleVisualize(context.Background(), nil, VisualizeInput{Ref: "db"})
	if err == nil || !strings.Contains(err.Error(), "at least one of 'sql' or 'chart_config' must be provided") {
		t.Fatalf("err = %v, want sql-or-chart_config-required error", err)
	}
}
