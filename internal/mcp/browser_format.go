package mcp

import (
	"fmt"
	"strings"

	"github.com/timescale/ghost/internal/common"
)

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
	return common.ResultSet{Columns: cols, Rows: stringRows, RowsAffected: int64(len(rows))}
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
	return strings.TrimRight(b.String(), "\n")
}
