package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// UIStateInput represents input for ghost_ui_state
type UIStateInput struct {
	Limit int `json:"limit,omitempty"`
}

func (UIStateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[UIStateInput](nil))
	schema.Properties["limit"].Description = fmt.Sprintf("Maximum number of result rows to return from the last query run. Defaults to %d.", defaultRowLimit)
	schema.Properties["limit"].Default = json.RawMessage(fmt.Sprintf("%d", defaultRowLimit))
	return schema
}

// UIStateOutput represents output for ghost_ui_state
type UIStateOutput struct {
	SelectedDatabaseID string             `json:"selected_database_id,omitempty"`
	EditorSQL          string             `json:"editor_sql,omitempty"`
	ChartConfig        string             `json:"chart_config,omitempty"`
	ResultView         string             `json:"result_view,omitempty"`
	LastRunStatus      string             `json:"last_run_status,omitempty"`
	LastRunError       string             `json:"last_run_error,omitempty"`
	ResultSets         []common.ResultSet `json:"result_sets,omitempty"`
	ChartError         string             `json:"chart_error,omitempty"`
	ChartDiagnostics   []ChartDiagnostic  `json:"chart_diagnostics,omitempty"`
}

func (UIStateOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[UIStateOutput](nil))
	schema.Properties["selected_database_id"].Description = "ID of the database currently selected in the UI"
	schema.Properties["editor_sql"].Description = "Current contents of the SQL editor"
	schema.Properties["chart_config"].Description = "Current chart config (the chart(data) function source)"
	schema.Properties["result_view"].Description = "Which view is active below the editor"
	schema.Properties["result_view"].Enum = []any{"table", "chart", "editor"}
	schema.Properties["last_run_status"].Description = "Status of the most recent query run"
	schema.Properties["last_run_status"].Examples = []any{"success", "failed", "running"}
	schema.Properties["last_run_error"].Description = "Error message from the most recent query run, if it failed"
	schema.Properties["chart_error"].Description = "Set when the last run's chart could not be rendered (e.g. an invalid chart config or data it can't plot). The run results are still returned; fix the chart config and retry to get an image."
	schema.Properties["chart_diagnostics"].Description = "Type and syntax issues the config editor found in the current chart config (the same errors a human sees as red squiggles). Each item has line, column, message, and severity ('error' or 'warning')."
	return schema
}

func newUIStateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_ui_state",
		Title:        "Get UI State",
		Description:  "Read the current state of the local web UI: the selected database, the SQL editor contents, the chart config, the active view, and the most recent query run's status and results (capped at 'limit' rows). When the last run succeeded, a rendered chart image of its data is also returned (drawn off-screen, regardless of the active view). Requires the local web UI (opens a browser if needed).",
		InputSchema:  UIStateInput{}.Schema(),
		OutputSchema: UIStateOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
			Title:         "Get UI State",
		},
	}
}

func (s *Server) handleUIState(ctx context.Context, req *mcp.CallToolRequest, input UIStateInput) (*mcp.CallToolResult, UIStateOutput, error) {
	if s.browser == nil {
		return nil, UIStateOutput{}, errors.New("the UI state tool is only available when running the MCP server locally (stdio transport)")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultRowLimit
	}

	var result uiStateResult
	if err := s.browser.request(ctx, commandUIState, uiStateCommand{Limit: limit}, &result); err != nil {
		return nil, UIStateOutput{}, fmt.Errorf("failed to read UI state: %w", err)
	}

	output := UIStateOutput{
		SelectedDatabaseID: result.SelectedDatabaseID,
		EditorSQL:          result.EditorSQL,
		ChartConfig:        result.ChartConfig,
		ResultView:         result.ResultView,
		ChartError:         result.ChartError,
		ChartDiagnostics:   toChartDiagnostics(result.ChartDiagnostics),
	}
	if result.LastRun != nil {
		output.LastRunStatus = result.LastRun.Status
		output.LastRunError = result.LastRun.Error
		if len(result.LastRun.Columns) > 0 || len(result.LastRun.Rows) > 0 {
			output.ResultSets = []common.ResultSet{browserResultSet(result.LastRun.Columns, result.LastRun.Rows)}
		}
	}

	content := []mcp.Content{&mcp.TextContent{Text: formatUIStateSummary(result) + chartDiagnosticsSuffix(result.ChartDiagnostics)}}
	if result.Image != "" {
		image, err := decodeImageDataURL(result.Image)
		if err != nil {
			return nil, UIStateOutput{}, err
		}
		content = append(content, image)
	}

	return &mcp.CallToolResult{Content: content}, output, nil
}

// formatUIStateSummary renders a short human-readable summary of the UI state
// for the tool's text content block.
func formatUIStateSummary(result uiStateResult) string {
	db := result.SelectedDatabaseID
	if db == "" {
		db = "(none)"
	}
	view := result.ResultView
	if view == "" {
		view = "(none)"
	}
	summary := fmt.Sprintf("Selected database: %s; view: %s", db, view)
	if result.LastRun != nil && result.LastRun.Status != "" {
		summary += fmt.Sprintf("; last run: %s (%d row(s))", result.LastRun.Status, result.LastRun.RowCount)
	}
	return summary
}
