package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStringifyCell(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		// A nil cell is a SQL NULL and must render as "NULL", matching
		// common.ExecuteQuery, not as an empty string (which is a distinct value).
		{name: "nil becomes NULL", in: nil, want: "NULL"},
		{name: "empty string stays empty", in: "", want: ""},
		{name: "string passes through", in: "hello", want: "hello"},
		{name: "number is formatted", in: 42, want: "42"},
		{name: "float is formatted", in: 1.5, want: "1.5"},
		// Booleans render in Postgres text format (t/f), matching
		// common.ExecuteQuery, not JSON's true/false.
		{name: "bool true becomes t", in: true, want: "t"},
		{name: "bool false becomes f", in: false, want: "f"},
		// Numbers arrive as json.Number (browser response decoded with
		// UseNumber). Its source literal must be preserved exactly — a large or
		// whole number must NOT be re-rendered in exponent form, as a float64
		// would (e.g. 10000000 -> "1e+07").
		{name: "json.Number large whole stays decimal", in: json.Number("10000000"), want: "10000000"},
		{name: "json.Number very large stays decimal", in: json.Number("1234567890123"), want: "1234567890123"},
		{name: "json.Number tiny stays decimal", in: json.Number("0.0000001"), want: "0.0000001"},
		{name: "json.Number float passes through", in: json.Number("1.5"), want: "1.5"},
		// JSON/JSONB cells arrive decoded as maps/slices; they must render as
		// valid JSON, not Go's debug format (e.g. not "map[a:b]").
		{name: "json object is marshaled", in: map[string]any{"a": "b"}, want: `{"a":"b"}`},
		{name: "json array is marshaled", in: []any{1.0, "x", true}, want: `[1,"x",true]`},
		{name: "nested json is marshaled", in: map[string]any{"k": []any{1.0, 2.0}}, want: `{"k":[1,2]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringifyCell(tt.in); got != tt.want {
				t.Errorf("stringifyCell(%#v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBrowserResultSet(t *testing.T) {
	columns := []browserColumn{{Name: "id", Type: "INT8"}, {Name: "name", Type: "TEXT"}}
	rows := [][]any{{1, "a"}, {2, nil}}

	got := browserResultSet(columns, rows, 5)

	if len(got.Columns) != 2 || got.Columns[0].Name != "id" || got.Columns[0].Type != "INT8" {
		t.Fatalf("unexpected columns: %+v", got.Columns)
	}
	// rowsAffected (the Postgres command-tag count) must be carried through, not
	// left at zero — otherwise the structured output would misreport it.
	if got.RowsAffected != 5 {
		t.Errorf("RowsAffected = %d, want 5", got.RowsAffected)
	}
	// The command-tag string is intentionally not carried for the browser path
	// (only a client-derived verb is available), so it stays empty rather than
	// being populated with a value that wouldn't match the server-side tag.
	if got.CommandTag != "" {
		t.Errorf("CommandTag = %q, want empty", got.CommandTag)
	}
	wantRows := [][]string{{"1", "a"}, {"2", "NULL"}}
	if len(got.Rows) != len(wantRows) {
		t.Fatalf("got %d rows, want %d", len(got.Rows), len(wantRows))
	}
	for i := range wantRows {
		for j := range wantRows[i] {
			if got.Rows[i][j] != wantRows[i][j] {
				t.Errorf("Rows[%d][%d] = %q, want %q", i, j, got.Rows[i][j], wantRows[i][j])
			}
		}
	}
}

func TestFormatVisualizeSummary_OmitsDiagnostics(t *testing.T) {
	result := visualizeResult{
		RunID:    "r1",
		Columns:  []browserColumn{{Name: "n"}},
		Rows:     [][]any{{1}},
		RowCount: 1,
		Image:    "data:image/png;base64,aGk=",
		ChartDiagnostics: []ChartDiagnostic{
			{Line: 1, Column: 1, Message: "bad key", Severity: "error"},
		},
	}
	got := formatVisualizeSummary(result, 50)
	if !strings.Contains(got, "A rendered chart image is attached below.") {
		t.Errorf("summary missing image line:\n%s", got)
	}
	// Diagnostics belong in the structured output (chart_diagnostics) and its
	// equivalent JSON text block, not the prose summary — echoing them here would
	// duplicate them in the tool result content.
	if strings.Contains(got, "bad key") {
		t.Errorf("summary should not echo diagnostics:\n%s", got)
	}
}
