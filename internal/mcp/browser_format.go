package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/timescale/ghost/internal/common"
)

// browserResultSet converts the browser's column/row representation into a
// [common.ResultSet] for the structured tool output. Cell values are stringified
// to match the server-side query path's [][]string row shape. rowsAffected is
// the Postgres command-tag count the browser reports for the run, matching
// common.ExecuteQuery's RowsAffected semantics (rows touched by a DML command,
// or rows returned by a SELECT). The command-tag string is intentionally not
// carried: unlike the server-side path (which has pgconn's real tag), the
// browser only has a client-derived command verb, so [common.ResultSet.CommandTag]
// is left empty rather than populated with a value that wouldn't match.
func browserResultSet(columns []browserColumn, rows [][]any, rowsAffected int64) common.ResultSet {
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
	return common.ResultSet{Columns: cols, Rows: stringRows, RowsAffected: rowsAffected}
}

// stringifyCell renders a JSON-decoded cell value as a string. A nil cell is a
// SQL NULL and becomes the literal "NULL" — matching common.ExecuteQuery's
// server-side path — so ghost_sql results don't depend on whether visualize was
// used, and a SQL NULL stays distinct from an empty string. Numbers arrive as
// json.Number (the browser response is decoded with UseNumber), whose String()
// is the exact source literal — so a large or whole number stays e.g.
// "10000000" rather than being re-rendered in exponent form ("1e+07") as a
// float64 would. Booleans render as Postgres's text representation ("t"/"f"),
// not JSON's "true"/"false", again matching the server-side path. Non-scalar
// values (JSON/JSONB objects and arrays, which the browser decodes into
// maps/slices) are re-marshaled to JSON so they read as valid JSON text (e.g.
// {"a":"b"}) rather than Go's debug format (e.g. map[a:b]); the marshal can't
// realistically fail for a JSON-decoded value, but if it did we fall back to
// fmt.Sprintf.
func stringifyCell(v any) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case string:
		return val
	case bool:
		// Postgres text format for booleans is t/f (what common.ExecuteQuery
		// returns by scanning into *string), not JSON's true/false.
		if val {
			return "t"
		}
		return "f"
	case json.Number:
		return val.String()
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
	lines := []string{fmt.Sprintf("Ran query in the browser UI: %d row(s) returned.", result.RowCount)}
	if len(result.Columns) > 0 {
		names := make([]string, len(result.Columns))
		for i, c := range result.Columns {
			names[i] = c.Name
		}
		lines = append(lines, fmt.Sprintf("Columns: %s", strings.Join(names, ", ")))
	}
	if result.RowCount > len(result.Rows) {
		lines = append(lines, fmt.Sprintf("Showing the first %d row(s); raise 'limit' (currently %d) or aggregate in SQL for more.", len(result.Rows), limit))
	}
	if result.Image != "" {
		lines = append(lines, "A rendered chart image is attached below.")
	}
	// The chart error and diagnostics are intentionally not echoed here: they're
	// carried in the structured output's chart_error/chart_diagnostics fields
	// (and the equivalent JSON text block), so repeating them in the prose
	// summary would duplicate them in the tool result content.
	return strings.Join(lines, "\n")
}
