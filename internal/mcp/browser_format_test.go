package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatChartDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		diagnostics []ChartDiagnostic
		want        string
	}{
		{
			name:        "none returns empty",
			diagnostics: nil,
			want:        "",
		},
		{
			name: "single error",
			diagnostics: []ChartDiagnostic{
				{Line: 2, Column: 5, Message: "Property 'seriess' does not exist", Severity: "error"},
			},
			want: "Chart config has 1 issue(s) reported by the editor (these may explain an unexpected chart even if it rendered):\n" +
				"  [error] line 2, col 5: Property 'seriess' does not exist",
		},
		{
			name: "multiple with empty severity defaults to error",
			diagnostics: []ChartDiagnostic{
				{Line: 1, Column: 1, Message: "a", Severity: "warning"},
				{Line: 3, Column: 2, Message: "b"},
			},
			want: "Chart config has 2 issue(s) reported by the editor (these may explain an unexpected chart even if it rendered):\n" +
				"  [warning] line 1, col 1: a\n" +
				"  [error] line 3, col 2: b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatChartDiagnostics(tt.diagnostics)
			if got != tt.want {
				t.Errorf("formatChartDiagnostics() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestChartDiagnosticsSuffix(t *testing.T) {
	if got := chartDiagnosticsSuffix(nil); got != "" {
		t.Errorf("chartDiagnosticsSuffix(nil) = %q, want empty", got)
	}
	got := chartDiagnosticsSuffix([]ChartDiagnostic{{Line: 1, Column: 1, Message: "x", Severity: "error"}})
	if !strings.HasPrefix(got, "\n") {
		t.Errorf("chartDiagnosticsSuffix() = %q, want leading newline", got)
	}
}

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

	got := browserResultSet(columns, rows, 5, "SELECT")

	if len(got.Columns) != 2 || got.Columns[0].Name != "id" || got.Columns[0].Type != "INT8" {
		t.Fatalf("unexpected columns: %+v", got.Columns)
	}
	// rowsAffected (the Postgres command-tag count) must be carried through, not
	// left at zero — otherwise the structured output would misreport it.
	if got.RowsAffected != 5 {
		t.Errorf("RowsAffected = %d, want 5", got.RowsAffected)
	}
	// commandTag must be carried through so visualized results report the same
	// command_tag as the server-side query path.
	if got.CommandTag != "SELECT" {
		t.Errorf("CommandTag = %q, want %q", got.CommandTag, "SELECT")
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

func TestFormatVisualizeSummary_WithDiagnostics(t *testing.T) {
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
	if !strings.Contains(got, "[error] line 1, col 1: bad key") {
		t.Errorf("summary missing diagnostics:\n%s", got)
	}
}
