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

// chartConfigDescriptionPrefix is the shared lead-in for the chart_config
// parameter description. It documents the expected function shape, the `data`
// contract, and points to the ECharts option reference so less capable models
// have the info they need (frontier models generally know ECharts already). The
// config is type-checked against the `EChartsOption` type in the UI's editor,
// and any issues are returned as chart_diagnostics, so the model gets corrective
// feedback after a first attempt.
const chartConfigDescriptionPrefix = "JavaScript source defining a function `chart(data)` that returns an Apache ECharts option object (see the ECharts option reference at https://echarts.apache.org/en/option.html). `data` provides `data.rows` (array of row objects keyed by column name) and `data.columns` ([{name, type}]). The UI's editor type-checks the config against the `EChartsOption` type and any issues are reported back as chart_diagnostics. "

// VisualizeInput represents input for ghost_visualize.
type VisualizeInput struct {
	Ref         string `json:"name_or_id,omitempty"`
	SQL         string `json:"sql,omitempty"`
	ChartConfig string `json:"chart_config,omitempty"`
	View        string `json:"view,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

func (VisualizeInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[VisualizeInput](nil))
	schema.Properties["name_or_id"].Description = "Database name or identifier to run the query against. Required when `sql` is provided; ignored when re-charting the previous run (no `sql`)."
	schema.Properties["sql"].Description = "SQL query to run in the local web UI. When provided, the query runs in the browser, the live UI updates so the user sees exactly what you ran, and the response includes the result rows (capped at `limit`) plus a rendered chart image. Omit to re-chart the most recent run with a new `chart_config` (without re-running the query). Multi-statement queries are supported; query parameters are not."
	schema.Properties["chart_config"].Description = chartConfigDescriptionPrefix + "When provided, it's applied to the run and the chart re-rendered. When omitted with `sql`, the previous or default config is used. You must provide at least one of `sql` or `chart_config`."
	schema.Properties["view"].Description = "Which view to show below the editor in the live UI. 'chart' renders the chart as the active view; 'table' shows the rows in a table. A chart image is returned regardless of the view (it's drawn off-screen). Defaults to 'chart'. Only applies when running a query (`sql`)."
	schema.Properties["view"].Enum = []any{"table", "chart"}
	schema.Properties["view"].Default = json.RawMessage(`"chart"`)
	schema.Properties["limit"].Description = fmt.Sprintf("Maximum number of result rows returned to you (the caller). Defaults to %d to conserve token usage. This caps only the rows returned in the response; the full result set is still computed and charted in the browser, so a small limit doesn't truncate the chart. Only applies when running a query (`sql`).", defaultRowLimit)
	schema.Properties["limit"].Default = json.RawMessage(fmt.Sprintf("%d", defaultRowLimit))
	return schema
}

// VisualizeOutput represents output for ghost_visualize. The rendered chart is
// returned as an image content block (exempt from the structured/text
// equivalence rule); these fields carry the machine-readable data and feedback.
type VisualizeOutput struct {
	// ResultSets carries the query results when a query was run (`sql`
	// provided). It is empty when only re-charting the previous run.
	ResultSets       []common.ResultSet `json:"result_sets,omitempty"`
	ChartError       string             `json:"chart_error,omitempty"`
	ChartDiagnostics []ChartDiagnostic  `json:"chart_diagnostics,omitempty"`
}

func (VisualizeOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[VisualizeOutput](nil))
	schema.Properties["result_sets"].Description = "Query results, returned when a query was run (`sql` provided). Empty when only re-charting the previous run."
	schema.Properties["chart_error"].Description = "Set when the chart could not be rendered (e.g. an invalid chart_config or data the config can't plot). The query (if any) still ran and its rows are returned; fix the chart_config and retry to get an image."
	schema.Properties["chart_diagnostics"].Description = "Type and syntax issues the web UI's config editor found in the chart_config (the same errors a human sees as red squiggles). May be present even when the chart rendered, since many type errors don't throw at runtime but still produce a wrong chart. Each item has line, column, message, and severity ('error' or 'warning')."
	return schema
}

func newVisualizeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_visualize",
		Title:        "Visualize Query",
		Description:  "Run a SQL query and/or render a chart in the local web UI, returning the result rows and a rendered chart image. Provide `sql` to run a query (and chart it); provide `chart_config` without `sql` to re-chart the most recent run with a new appearance (without re-running it). Use this when you want the user to see the results — it runs in a live browser UI showing exactly what you ran, with the rows and a chart. For non-user-facing queries (e.g. autonomous work where the user doesn't need to watch), use ghost_sql instead. Requires the local web UI (opens a browser if needed).",
		InputSchema:  VisualizeInput{}.Schema(),
		OutputSchema: VisualizeOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  false,
			OpenWorldHint:   new(true),
			Title:           "Visualize Query",
		},
	}
}

func (s *Server) handleVisualize(ctx context.Context, req *mcp.CallToolRequest, input VisualizeInput) (*mcp.CallToolResult, VisualizeOutput, error) {
	if s.browser == nil {
		return nil, VisualizeOutput{}, errors.New("visualization is only available when running the MCP server locally (stdio transport)")
	}
	if input.SQL == "" && input.ChartConfig == "" {
		return nil, VisualizeOutput{}, errors.New("at least one of 'sql' or 'chart_config' must be provided")
	}

	// No SQL: re-chart the most recent run with a new config (the chart_config
	// requirement is guaranteed by the check above).
	if input.SQL == "" {
		return s.handleVisualizeChart(ctx, input)
	}
	return s.handleVisualizeQuery(ctx, input)
}

// handleVisualizeQuery runs the query in the browser and returns the result
// rows (capped at limit) plus a rendered chart image (and any
// chart_error/chart_diagnostics).
func (s *Server) handleVisualizeQuery(ctx context.Context, input VisualizeInput) (*mcp.CallToolResult, VisualizeOutput, error) {
	_, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, VisualizeOutput{}, err
	}
	if input.Ref == "" {
		return nil, VisualizeOutput{}, errors.New("'name_or_id' is required when running a query")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultRowLimit
	}
	view := input.View
	if view == "" {
		view = "chart"
	}

	// Resolve the ref (which may be a database name or id) to the canonical
	// database id before dispatching to the browser. The web UI selects the
	// database by id and reflects it in the URL (?db=<id>); if we passed a name
	// through, the selector wouldn't match any option ("Select a database...")
	// and the URL would show the name. The backend always has the API client to
	// resolve this reliably, whereas the frontend's database list may not be
	// loaded yet.
	database, err := resolveDatabase(ctx, client, projectID, input.Ref)
	if err != nil {
		return nil, VisualizeOutput{}, err
	}

	// Fail fast on a database that can't accept connections, with the same
	// friendly guidance as the server-side path — rather than opening a browser,
	// waiting for a client, and surfacing a raw connection failure as a generic
	// "visualization failed". The in-process serve handler also checks this, but
	// only after we've dispatched the command.
	if err := common.CheckReady(database); err != nil {
		return nil, VisualizeOutput{}, handleDatabaseError(err)
	}

	var result visualizeResult
	err = s.browser.request(ctx, commandVisualize, visualizeCommand{
		DatabaseRef: database.Id,
		SQL:         input.SQL,
		View:        view,
		ChartConfig: input.ChartConfig,
		Limit:       limit,
	}, &result)
	if err != nil {
		return nil, VisualizeOutput{}, fmt.Errorf("visualization failed: %w", err)
	}

	output := VisualizeOutput{
		ResultSets:       []common.ResultSet{browserResultSet(result.Columns, result.Rows, result.RowsAffected)},
		ChartError:       result.ChartError,
		ChartDiagnostics: result.ChartDiagnostics,
	}

	// We set Content explicitly (a human-readable summary plus, optionally, the
	// rendered chart image), which opts out of the SDK auto-populating it with
	// the structured output's JSON. Prepend that JSON ourselves so the result
	// rows stay visible to clients that read only the text content (per the MCP
	// spec, structured and unstructured content must be equivalent).
	structured, err := structuredOutputContent(output)
	if err != nil {
		return nil, VisualizeOutput{}, err
	}
	content := []mcp.Content{
		&mcp.TextContent{Text: formatVisualizeSummary(result, limit)},
		structured,
	}
	if result.Image != "" {
		image, err := decodeImageDataURL(result.Image)
		if err != nil {
			return nil, VisualizeOutput{}, err
		}
		content = append(content, image)
	}

	return &mcp.CallToolResult{Content: content}, output, nil
}

// handleVisualizeChart reapplies a chart config to the most recent run and
// re-renders it, without running a query. Returns the rendered image (and any
// chart_error/chart_diagnostics); no result sets, since no query was run.
func (s *Server) handleVisualizeChart(ctx context.Context, input VisualizeInput) (*mcp.CallToolResult, VisualizeOutput, error) {
	var result chartResult
	cmd := chartCommand{ChartConfig: input.ChartConfig}
	if err := s.browser.request(ctx, commandChart, cmd, &result); err != nil {
		return nil, VisualizeOutput{}, fmt.Errorf("charting failed: %w", err)
	}

	image, err := decodeImageDataURL(result.Image)
	if err != nil {
		return nil, VisualizeOutput{}, err
	}

	output := VisualizeOutput{
		ChartError:       result.ChartError,
		ChartDiagnostics: result.ChartDiagnostics,
	}
	structured, err := structuredOutputContent(output)
	if err != nil {
		return nil, VisualizeOutput{}, err
	}

	// A render failure (bad config or unplottable data) is not a tool error: the
	// chart config was applied to the UI, and the agent needs the error message
	// plus the editor diagnostics to fix it — matching the query path, which
	// reports chartError rather than failing the call.
	if image == nil {
		chartError := result.ChartError
		if chartError == "" {
			chartError = "the chart could not be rendered"
		}
		summary := "Applied chart config to the last run, but the chart could not be rendered: " + chartError
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: summary}, structured},
		}, output, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Applied chart config to the last run. Rendered chart attached."},
			structured,
			image,
		},
	}, output, nil
}
