package mcp

import (
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
// to match the server-side query path's [][]string row shape.
func browserResultSet(columns []browserColumn, rows [][]any) common.ResultSet {
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
	// RowsAffected is left at zero: it reflects a Postgres command tag (rows
	// touched by a DML command), which the browser widget doesn't report. The
	// returned row count is conveyed separately (RowCount / len(rows)), so we
	// don't conflate it with RowsAffected here.
	return common.ResultSet{Columns: cols, Rows: stringRows}
}

// stringifyCell renders a JSON-decoded cell value as a string. nil becomes the
// empty string (mirroring NULL handling elsewhere).
func stringifyCell(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
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
	} else if result.ChartError != "" {
		fmt.Fprintf(&b, "The chart could not be rendered: %s\n", result.ChartError)
	}
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
