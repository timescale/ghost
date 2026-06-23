package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/timescale/ghost/internal/common"
)

// ChartDiagnostic is one type/syntax issue the web UI's config editor reports
// for a chart config, surfaced in structured tool output.
type ChartDiagnostic struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// toChartDiagnostics converts the browser's wire diagnostics into the structured
// output type, returning nil when there are none.
func toChartDiagnostics(diagnostics []chartDiagnostic) []ChartDiagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	out := make([]ChartDiagnostic, len(diagnostics))
	for i, d := range diagnostics {
		out[i] = ChartDiagnostic(d)
	}
	return out
}

// browserResultSet converts the browser's column/row representation into a
// [common.ResultSet] for the structured tool output. Cell values are stringified
// to match the server-side query path's [][]string row shape. rowsAffected is
// the Postgres command-tag count the browser reports for the run, matching
// common.ExecuteQuery's RowsAffected semantics (rows touched by a DML command,
// or rows returned by a SELECT) — so the structured output is accurate whether
// or not the query was visualized. commandTag is the Postgres command tag (e.g.
// "SELECT"), matching common.ResultSet.CommandTag from the server-side path.
func browserResultSet(columns []browserColumn, rows [][]any, rowsAffected int64, commandTag string) common.ResultSet {
	cols := make([]common.Column, len(columns))
	for i, c := range columns {
		cols[i] = common.Column{Name: c.Name, Type: c.Type}
	}
	stringRows := make([][]string, len(rows))
	for i, row := range rows {
		cells := make([]string, len(row))
		for j, v := range row {
			cells[j] = stringifyCell(v)
		}
		stringRows[i] = cells
	}
	return common.ResultSet{CommandTag: commandTag, Columns: cols, Rows: stringRows, RowsAffected: rowsAffected}
}

// stringifyCell renders a JSON-decoded cell value as a string. A nil cell is a
// SQL NULL and becomes the literal "NULL" — matching common.ExecuteQuery's
// server-side path — so ghost_sql results don't depend on whether visualize was
// used, and a SQL NULL stays distinct from an empty string. Non-scalar values
// (JSON/JSONB objects and arrays, which the browser decodes into maps/slices)
// are re-marshaled to JSON so they read as valid JSON text (e.g. {"a":"b"})
// rather than Go's debug format (e.g. map[a:b]); the marshal can't realistically
// fail for a JSON-decoded value, but if it did we fall back to fmt.Sprintf.
func stringifyCell(v any) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case string:
		return val
	case map[string]any, []any:
		if b, err := json.Marshal(val); err == nil {
			return string(b)
		}
	}
	return fmt.Sprintf("%v", v)
}

// formatVisualizeSummary describes the outcome of a visualize run for the agent:
// the row count, the columns, and whether (and how many of) the rows were
// returned.
func formatVisualizeSummary(result visualizeResult, limit int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ran query in the browser UI: %d row(s) returned.\n", result.RowCount)
	if len(result.Columns) > 0 {
		names := make([]string, len(result.Columns))
		for i, c := range result.Columns {
			names[i] = c.Name
		}
		fmt.Fprintf(&b, "Columns: %s\n", strings.Join(names, ", "))
	}
	if result.RowCount > len(result.Rows) {
		fmt.Fprintf(&b, "Showing the first %d row(s); raise 'limit' (currently %d) or aggregate in SQL for more.\n", len(result.Rows), limit)
	}
	if result.Image != "" {
		b.WriteString("A rendered chart image is attached below.\n")
	}
	// The chart error is intentionally not echoed here: it's carried in the
	// structured output's chart_error field, so repeating it in the prose summary
	// would duplicate it in the tool result content.
	if diag := formatChartDiagnostics(result.ChartDiagnostics); diag != "" {
		b.WriteString(diag)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatChartDiagnostics renders the chart config's editor diagnostics (type
// and syntax errors) as a short block for the agent, or "" if there are none.
// These are the same issues a human sees as squiggles in the config editor and
// often explain a wrong-looking chart that still rendered without throwing.
func formatChartDiagnostics(diagnostics []chartDiagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Chart config has %d issue(s) reported by the editor (these may explain an unexpected chart even if it rendered):\n", len(diagnostics))
	for _, d := range diagnostics {
		severity := d.Severity
		if severity == "" {
			severity = "error"
		}
		fmt.Fprintf(&b, "  [%s] line %d, col %d: %s\n", severity, d.Line, d.Column, d.Message)
	}
	return strings.TrimRight(b.String(), "\n")
}

// chartDiagnosticsSuffix returns a leading-newline-prefixed diagnostics block to
// append to a summary line, or "" if there are no diagnostics.
func chartDiagnosticsSuffix(diagnostics []chartDiagnostic) string {
	if diag := formatChartDiagnostics(diagnostics); diag != "" {
		return "\n" + diag
	}
	return ""
}
