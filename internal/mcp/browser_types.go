package mcp

// This file defines the command payloads and response shapes exchanged with
// the browser over the agent bridge. The web orchestrator (web/src/agent/)
// mirrors these JSON shapes — keep them in sync.

// browserCommand identifies an agent command type dispatched to the browser.
type browserCommand string

// Agent command types dispatched to the browser.
const (
	commandVisualize browserCommand = "visualize"
	commandChart     browserCommand = "chart"
	commandUIState   browserCommand = "uiState"
)

// visualizeCommand runs a query in the browser, syncs the live UI, and
// optionally applies a chart config and renders a chart.
type visualizeCommand struct {
	DatabaseRef string `json:"databaseRef"`
	SQL         string `json:"sql"`
	ChartConfig string `json:"chartConfig,omitempty"`
	Limit       int    `json:"limit"`
}

// chartCommand reapplies a chart config to the last run and re-renders it.
type chartCommand struct {
	ChartConfig string `json:"chartConfig"`
}

// uiStateCommand reads the current UI state, capping returned rows at Limit.
type uiStateCommand struct {
	Limit int `json:"limit"`
}

// browserColumn describes one column of a result set returned by the browser.
type browserColumn struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// ChartDiagnostic is one type/syntax issue reported by the editor's language
// service for a chart config (the same squiggles a human sees in the editor).
// It is both the browser's wire shape and the type surfaced in structured tool
// output — the JSON tags happen to match, so no conversion is needed.
type ChartDiagnostic struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // "error" | "warning"
}

// visualizeResult is the browser's response to a visualize command.
type visualizeResult struct {
	RunID   string          `json:"runId"`
	Columns []browserColumn `json:"columns"`
	Rows    [][]any         `json:"rows"`
	// RowCount is the true total number of rows the query produced, independent
	// of any row cap applied when reading Rows back for the agent.
	RowCount int `json:"rowCount"`
	// RowsAffected is the Postgres command-tag count (rows touched by a DML
	// command, or rows returned by a SELECT), matching common.ExecuteQuery's
	// server-side semantics.
	RowsAffected int64 `json:"rowsAffected"`
	// Image is a data URL (e.g. "data:image/png;base64,...") of the rendered
	// chart. Present when the chart rendered successfully.
	Image string `json:"image,omitempty"`
	// ChartError explains why the chart couldn't be rendered (bad config or
	// unplottable data). The run data is still returned alongside it. Mutually
	// exclusive with Image.
	ChartError string `json:"chartError,omitempty"`
	// ChartDiagnostics are type/syntax issues the editor's language service
	// found in the chart config. May be present even when the chart rendered.
	ChartDiagnostics []ChartDiagnostic `json:"chartDiagnostics,omitempty"`
}

// chartResult is the browser's response to a chart command.
type chartResult struct {
	// Image is a data URL of the rendered chart. Present when the chart rendered
	// successfully; mutually exclusive with ChartError.
	Image string `json:"image,omitempty"`
	// ChartError explains why the chart couldn't be rendered (bad config or
	// unplottable data). Mutually exclusive with Image.
	ChartError       string            `json:"chartError,omitempty"`
	ChartDiagnostics []ChartDiagnostic `json:"chartDiagnostics,omitempty"`
}

// lastRunState describes the most recent query run in the browser UI.
type lastRunState struct {
	RunID  string `json:"runId,omitempty"`
	Status string `json:"status,omitempty"`
	// RowCount is the true total number of rows the query produced, independent
	// of any row cap applied when reading Rows back for the agent.
	RowCount int `json:"rowCount"`
	// RowsAffected is the Postgres command-tag count (see visualizeResult).
	RowsAffected int64           `json:"rowsAffected"`
	Columns      []browserColumn `json:"columns,omitempty"`
	Rows         [][]any         `json:"rows,omitempty"`
	Error        string          `json:"error,omitempty"`
}

// uiStateResult is the browser's response to a uiState command.
type uiStateResult struct {
	SelectedDatabaseID string        `json:"selectedDatabaseId,omitempty"`
	EditorSQL          string        `json:"editorSql,omitempty"`
	ChartConfig        string        `json:"chartConfig,omitempty"`
	ResultView         string        `json:"resultView,omitempty"`
	LastRun            *lastRunState `json:"lastRun,omitempty"`
	// Image is a data URL of the rendered chart of the last run. Present when
	// the chart rendered successfully.
	Image string `json:"image,omitempty"`
	// ChartError explains why the chart couldn't be rendered, if applicable.
	// Mutually exclusive with Image.
	ChartError string `json:"chartError,omitempty"`
	// ChartDiagnostics are type/syntax issues the editor's language service
	// found in the chart config.
	ChartDiagnostics []ChartDiagnostic `json:"chartDiagnostics,omitempty"`
}
