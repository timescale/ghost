package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/util"
)

// defaultRowLimit caps how many rows are returned to the agent by ghost_sql.
// This prevents a large result set (potentially millions of rows) from being
// dumped into the LLM's context. Callers can raise it via the limit parameter.
const defaultRowLimit = 50

// SQLInput represents input for ghost_sql
type SQLInput struct {
	Ref         string   `json:"name_or_id"`
	Query       string   `json:"query,omitempty"`
	File        string   `json:"file,omitempty"`
	Parameters  []string `json:"parameters,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Visualize   string   `json:"visualize,omitempty"`
	ChartConfig string   `json:"chart_config,omitempty"`
}

func (SQLInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[SQLInput](nil))
	databaseRefInputProperties(schema)
	schema.Properties["query"].Description = "SQL query to execute. Multi-statement queries are supported when no parameters are provided. Exactly one of 'query' or 'file' must be provided."
	schema.Properties["file"].Description = "Path to a SQL file on disk to execute. Multi-statement files are supported when no parameters are provided. Exactly one of 'query' or 'file' must be provided."
	schema.Properties["parameters"].Description = "Query parameters. Values are substituted for $1, $2, etc. placeholders in the query. Only supported for single-statement queries"
	schema.Properties["limit"].Description = fmt.Sprintf("Maximum number of result rows to return to you (the caller). Defaults to %d. This caps how much data is returned to avoid overwhelming the context window — it does NOT add a LIMIT to your SQL, so aggregate or add LIMIT in the query itself if you need to bound what the database computes. Raise this only when you genuinely need more rows.", defaultRowLimit)
	schema.Properties["limit"].Default = json.RawMessage(fmt.Sprintf("%d", defaultRowLimit))
	schema.Properties["visualize"].Description = "Render the results in the local web UI instead of (or in addition to) returning them as text. 'table' shows the rows in a table; 'chart' renders a chart (provide chart_config) as the active view. In BOTH cases the response includes a PNG image of the rendered chart so you can inspect the data visually; if the chart can't be rendered, the query still succeeds and 'chart_error' explains why. When set, the query runs in the browser and the live UI is updated so the user sees exactly what you ran. Opens a browser if one isn't already connected. Omit to just run server-side and return rows as text (no image)."
	schema.Properties["visualize"].Enum = []any{"table", "chart"}
	schema.Properties["chart_config"].Description = "JavaScript source defining a function `chart(data)` that returns an Apache ECharts option object. `data` provides `data.rows` (array of row objects keyed by column name) and `data.columns` ([{name, type}]). Used with either visualize view to render the returned chart image; with visualize='chart' it also becomes the active view. Applied to the chart and shown in the UI's config editor (overwriting any existing config). If omitted, a sensible default chart config is used."
	return schema
}

// SQLOutput represents output for ghost_sql
type SQLOutput struct {
	ResultSets       []common.ResultSet `json:"result_sets"`
	ExecutionTime    string             `json:"execution_time,omitempty"`
	ChartError       string             `json:"chart_error,omitempty"`
	ChartDiagnostics []ChartDiagnostic  `json:"chart_diagnostics,omitempty"`
}

func (SQLOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[SQLOutput](nil))
	schema.Properties["execution_time"].Description = "Total client-side elapsed time for all statements"
	schema.Properties["chart_error"].Description = "Set when visualization was requested but the chart could not be rendered (e.g. an invalid chart_config or data the config can't plot). The query still ran and its rows are returned; fix the chart_config and retry to get an image."
	schema.Properties["chart_diagnostics"].Description = "Type and syntax issues the web UI's config editor found in the chart_config (the same errors a human sees as red squiggles). May be present even when the chart rendered, since many type errors don't throw at runtime but still produce a wrong chart. Each item has line, column, message, and severity ('error' or 'warning')."
	return schema
}

func newSQLTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "ghost_sql",
		Title:        "Execute SQL",
		Description:  "Execute a SQL query against a database. If the connection fails, the database may not be running - use ghost_list to check its status.",
		InputSchema:  SQLInput{}.Schema(),
		OutputSchema: SQLOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  false,
			OpenWorldHint:   new(true),
			Title:           "Execute SQL",
		},
	}
}

func (s *Server) handleSQL(ctx context.Context, req *mcp.CallToolRequest, input SQLInput) (*mcp.CallToolResult, SQLOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, SQLOutput{}, err
	}

	if (input.Query == "") == (input.File == "") {
		return nil, SQLOutput{}, errors.New("exactly one of 'query' or 'file' must be provided")
	}

	query := input.Query
	if input.File != "" {
		data, err := os.ReadFile(util.ExpandPath(input.File))
		if err != nil {
			return nil, SQLOutput{}, fmt.Errorf("failed to read SQL file: %w", err)
		}
		query = string(data)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultRowLimit
	}

	// Visualization runs the query in the browser (not server-side) so the live
	// UI reflects exactly what the agent ran, and the chart can be rendered and
	// screenshotted. Gated to local (stdio) mode where a browser can be opened.
	if input.Visualize != "" {
		if s.browser == nil {
			return nil, SQLOutput{}, errors.New("visualization is only available when running the MCP server locally (stdio transport)")
		}
		return s.handleSQLVisualize(ctx, client, projectID, input, query, limit)
	}

	// Execute the query
	result, err := common.ExecuteQuery(ctx, common.ExecuteQueryArgs{
		Client:      client,
		ProjectID:   projectID,
		DatabaseRef: input.Ref,
		Query:       query,
		Role:        "tsdbadmin",
		Parameters:  input.Parameters,
		ReadOnly:    cfg.ReadOnly,
	})
	if err != nil {
		return nil, SQLOutput{}, handleDatabaseError(err)
	}

	capResultSetRows(result.ResultSets, limit)

	return nil, SQLOutput{
		ResultSets:    result.ResultSets,
		ExecutionTime: result.ExecutionTime.String(),
	}, nil
}

// handleSQLVisualize runs the query in the browser and returns a compact
// summary (capped rows + columns) plus a rendered chart image. The image is
// produced for both 'table' and 'chart' views (the browser renders it
// off-screen); it may be omitted only if the data can't be charted by the
// active config in the non-chart case.
func (s *Server) handleSQLVisualize(ctx context.Context, client api.ClientWithResponsesInterface, projectID string, input SQLInput, query string, limit int) (*mcp.CallToolResult, SQLOutput, error) {
	if len(input.Parameters) > 0 {
		return nil, SQLOutput{}, errors.New("query parameters are not supported with visualize")
	}

	// Resolve the ref (which may be a database name or id) to the canonical
	// database id before dispatching to the browser. The web UI selects the
	// database by id and reflects it in the URL (?db=<id>); if we passed a name
	// through, the selector wouldn't match any option ("Select a database...")
	// and the URL would show the name. The backend always has the API client to
	// resolve this reliably, whereas the frontend's database list may not be
	// loaded yet.
	databaseID, err := resolveDatabaseID(ctx, client, projectID, input.Ref)
	if err != nil {
		return nil, SQLOutput{}, err
	}

	var result visualizeResult
	err = s.browser.request(ctx, commandVisualize, visualizeCommand{
		DatabaseRef: databaseID,
		SQL:         query,
		View:        input.Visualize,
		ChartConfig: input.ChartConfig,
		Limit:       limit,
	}, &result)
	if err != nil {
		return nil, SQLOutput{}, fmt.Errorf("visualization failed: %w", err)
	}

	output := SQLOutput{
		ResultSets:       []common.ResultSet{browserResultSet(result.Columns, result.Rows, result.RowsAffected)},
		ChartError:       result.ChartError,
		ChartDiagnostics: toChartDiagnostics(result.ChartDiagnostics),
	}

	// We set Content explicitly (a human-readable summary plus, optionally, the
	// rendered chart image), which opts out of the SDK auto-populating it with
	// the structured output's JSON. Prepend that JSON ourselves so the result
	// rows stay visible to clients that read only the text content (per the MCP
	// spec, structured and unstructured content must be equivalent).
	structured, err := structuredOutputContent(output)
	if err != nil {
		return nil, SQLOutput{}, err
	}
	content := []mcp.Content{
		&mcp.TextContent{Text: formatVisualizeSummary(result, limit)},
		structured,
	}
	if result.Image != "" {
		image, err := decodeImageDataURL(result.Image)
		if err != nil {
			return nil, SQLOutput{}, err
		}
		content = append(content, image)
	}

	return &mcp.CallToolResult{Content: content}, output, nil
}

// capResultSetRows truncates each result set's rows to at most limit, so large
// result sets aren't returned wholesale to the agent. RowsAffected is left
// untouched: it reflects the Postgres command tag (e.g. rows touched by an
// UPDATE/DELETE), not the number of result rows returned, so it must not be
// overwritten with a post-truncation count.
func capResultSetRows(sets []common.ResultSet, limit int) {
	for i := range sets {
		if len(sets[i].Rows) > limit {
			sets[i].Rows = sets[i].Rows[:limit]
		}
	}
}
