package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/util"
)

// chartConfigDescriptionPrefix is the shared lead-in for the chart_config
// parameter description on both ghost_chart and ghost_sql. It documents the
// expected function shape, the `data` contract, and points to the ECharts
// option reference so less capable models have the info they need (frontier
// models generally know ECharts already). The config is type-checked against
// the `EChartsOption` type in the UI's editor, and any issues are returned as
// chart_diagnostics, so the model gets corrective feedback after a first attempt.
const chartConfigDescriptionPrefix = "JavaScript source defining a function `chart(data)` that returns an Apache ECharts option object (see the ECharts option reference at https://echarts.apache.org/en/option.html). `data` provides `data.rows` (array of row objects keyed by column name) and `data.columns` ([{name, type}]). The UI's editor type-checks the config against the `EChartsOption` type and any issues are reported back as chart_diagnostics. "

// ChartInput represents input for ghost_chart
type ChartInput struct {
	ChartConfig string `json:"chart_config"`
}

func (ChartInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ChartInput](nil))
	schema.Properties["chart_config"].Description = chartConfigDescriptionPrefix + "Applied to the most recent query run's results — it does NOT run a query (use ghost_sql with visualize='chart' to run a query and chart it). The config is shown in the UI's editor (overwriting any existing config), and the response includes a PNG image of the rendered chart so you can verify its appearance."
	return schema
}

// ChartOutput represents output for ghost_chart. The rendered chart is returned
// as an image content block (exempt from the structured/text equivalence rule);
// these fields carry the machine-readable feedback, matching the chart fields
// of ghost_sql and ghost_ui_state.
type ChartOutput struct {
	ChartError       string            `json:"chart_error,omitempty"`
	ChartDiagnostics []ChartDiagnostic `json:"chart_diagnostics,omitempty"`
}

func (ChartOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ChartOutput](nil))
	schema.Properties["chart_error"].Description = "Set when the chart could not be rendered (e.g. an invalid chart_config or data the config can't plot). The config was still applied to the UI; fix it and retry to get an image."
	schema.Properties["chart_diagnostics"].Description = "Type and syntax issues the web UI's config editor found in the chart_config (the same errors a human sees as red squiggles). May be present even when the chart rendered, since many type errors don't throw at runtime but still produce a wrong chart. Each item has line, column, message, and severity ('error' or 'warning')."
	return schema
}

func newChartTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_chart",
		Title:        "Configure Chart",
		Description:  "Apply an Apache ECharts config to the results of the most recent query run in the local web UI, and return a rendered image of the chart. Use this to iterate on a chart's appearance without re-running the query. To run a new query and chart it, use ghost_sql with visualize='chart'. Requires the local web UI (opens a browser if needed).",
		InputSchema:  ChartInput{}.Schema(),
		OutputSchema: ChartOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  false,
			OpenWorldHint: new(true),
			Title:         "Configure Chart",
		},
	}
}

func (s *Server) handleChart(ctx context.Context, req *mcp.CallToolRequest, input ChartInput) (*mcp.CallToolResult, ChartOutput, error) {
	if s.browser == nil {
		return nil, ChartOutput{}, errors.New("charting is only available when running the MCP server locally (stdio transport)")
	}
	// The API client is verified inside browser.ensureStarted before the server
	// is started or the browser opened, so a logged-out user gets the real
	// auth/config error rather than an opaque "no browser connected" timeout.
	if input.ChartConfig == "" {
		return nil, ChartOutput{}, errors.New("chart_config is required")
	}

	var result chartResult
	// chartCommand and ChartInput hold the same field but with different JSON
	// tags — the browser's camelCase wire format (chartConfig) vs the MCP input
	// convention (chart_config) — so they're deliberately distinct types. Go
	// ignores struct tags when converting, so the cast re-tags the value for the
	// browser wire format (staticcheck S1016 prefers this over a field-by-field
	// literal).
	if err := s.browser.request(ctx, commandChart, chartCommand(input), &result); err != nil {
		return nil, ChartOutput{}, fmt.Errorf("charting failed: %w", err)
	}

	image, err := decodeImageDataURL(result.Image)
	if err != nil {
		return nil, ChartOutput{}, err
	}

	output := ChartOutput{
		ChartError:       result.ChartError,
		ChartDiagnostics: result.ChartDiagnostics,
	}

	// We set Content explicitly (a human-readable summary plus, on success, the
	// rendered chart image), which opts out of the SDK auto-populating it with
	// the structured output's JSON. Prepend that JSON ourselves so the
	// chart_error/chart_diagnostics stay visible to clients that read only the
	// text content (per the MCP spec, structured and unstructured content must
	// be equivalent).
	structured, err := structuredOutputContent(output)
	if err != nil {
		return nil, ChartOutput{}, err
	}

	// A render failure (bad config or unplottable data) is not a tool error: the
	// chart config was applied to the UI, and the agent needs the error message
	// plus the editor diagnostics to fix it — matching the ghost_sql visualize
	// path, which reports chartError rather than failing the call.
	if image == nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Applied chart config to the last run, but the chart could not be rendered"},
				structured,
			},
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
