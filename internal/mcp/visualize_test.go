package mcp

import (
	"context"
	"strings"
	"testing"
)

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
