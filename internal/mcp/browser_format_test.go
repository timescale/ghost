package mcp

import (
	"strings"
	"testing"
)

func TestFormatChartDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		diagnostics []chartDiagnostic
		want        string
	}{
		{
			name:        "none returns empty",
			diagnostics: nil,
			want:        "",
		},
		{
			name: "single error",
			diagnostics: []chartDiagnostic{
				{Line: 2, Column: 5, Message: "Property 'seriess' does not exist", Severity: "error"},
			},
			want: "Chart config has 1 issue(s) reported by the editor (these may explain an unexpected chart even if it rendered):\n" +
				"  [error] line 2, col 5: Property 'seriess' does not exist",
		},
		{
			name: "multiple with empty severity defaults to error",
			diagnostics: []chartDiagnostic{
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
	got := chartDiagnosticsSuffix([]chartDiagnostic{{Line: 1, Column: 1, Message: "x", Severity: "error"}})
	if !strings.HasPrefix(got, "\n") {
		t.Errorf("chartDiagnosticsSuffix() = %q, want leading newline", got)
	}
}

func TestToChartDiagnostics(t *testing.T) {
	if got := toChartDiagnostics(nil); got != nil {
		t.Errorf("toChartDiagnostics(nil) = %v, want nil", got)
	}
	in := []chartDiagnostic{{Line: 4, Column: 7, Message: "msg", Severity: "warning"}}
	got := toChartDiagnostics(in)
	want := ChartDiagnostic{Line: 4, Column: 7, Message: "msg", Severity: "warning"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("toChartDiagnostics() = %+v, want [%+v]", got, want)
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
		{name: "bool is formatted", in: true, want: "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringifyCell(tt.in); got != tt.want {
				t.Errorf("stringifyCell(%#v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatVisualizeSummary_WithDiagnostics(t *testing.T) {
	result := visualizeResult{
		RunID:    "r1",
		Columns:  []browserColumn{{Name: "n"}},
		Rows:     [][]any{{1}},
		RowCount: 1,
		Image:    "data:image/png;base64,aGk=",
		ChartDiagnostics: []chartDiagnostic{
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
