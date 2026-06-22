package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/timescale/ghost/internal/common"
)

func TestStructuredOutputContent(t *testing.T) {
	// The block's text must be the exact JSON marshaling of the output, so a
	// client reading only the text content sees the same payload as
	// StructuredContent (per the MCP spec's equivalence requirement). This
	// mirrors what the go-sdk emits automatically when Content is left unset.
	output := SQLOutput{
		ResultSets: []common.ResultSet{{
			Columns:      []common.Column{{Name: "id", Type: "int4"}},
			Rows:         [][]string{{"1"}, {"NULL"}},
			RowsAffected: 2,
		}},
		ExecutionTime: "5ms",
	}

	block, err := structuredOutputContent(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshaling expected output: %v", err)
	}
	if block.Text != string(want) {
		t.Errorf("Text =\n%s\nwant\n%s", block.Text, want)
	}

	// The serialized rows (including the NULL sentinel and rows_affected) must be
	// present so the model actually sees the data, not just a summary.
	for _, substr := range []string{`"rows_affected":2`, `"NULL"`, `"execution_time":"5ms"`} {
		if !strings.Contains(block.Text, substr) {
			t.Errorf("Text missing %q:\n%s", substr, block.Text)
		}
	}
}
