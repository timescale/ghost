package mcp

// This file defines the command payloads and response shapes exchanged with
// the browser over the agent bridge. The web orchestrator (web/src/agent/)
// mirrors these JSON shapes — keep them in sync.

// Agent command types dispatched to the browser.
const (
	commandVisualize = "visualize"
	commandChart     = "chart"
	commandUIState   = "uiState"
)

// visualizeCommand runs a query in the browser, syncs the live UI, and
// optionally applies a chart config and renders a chart.
type visualizeCommand struct {
	DatabaseRef string `json:"databaseRef"`
	SQL         string `json:"sql"`
	View        string `json:"view"` // "table" | "chart"
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

// visualizeResult is the browser's response to a visualize command.
type visualizeResult struct {
	RunID    string          `json:"runId"`
	Columns  []browserColumn `json:"columns"`
	Rows     [][]any         `json:"rows"`
	RowCount int             `json:"rowCount"`
	// Image is a data URL (e.g. "data:image/png;base64,...") of the rendered
	// chart. Present when the chart rendered successfully.
	Image string `json:"image,omitempty"`
	// ChartError explains why the chart couldn't be rendered (bad config or
	// unplottable data). The run data is still returned alongside it. Mutually
	// exclusive with Image.
	ChartError string `json:"chartError,omitempty"`
}

// chartResult is the browser's response to a chart command.
type chartResult struct {
	Image string `json:"image"`
}

// lastRunState describes the most recent query run in the browser UI.
type lastRunState struct {
	RunID    string          `json:"runId,omitempty"`
	Status   string          `json:"status,omitempty"`
	RowCount int             `json:"rowCount"`
	Columns  []browserColumn `json:"columns,omitempty"`
	Rows     [][]any         `json:"rows,omitempty"`
	Error    string          `json:"error,omitempty"`
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
}
