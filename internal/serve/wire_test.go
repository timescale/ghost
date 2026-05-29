package serve

import (
	"encoding/json"
	"testing"
)

func TestExecuteQueryRequest_SQL(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "statements array (widget canonical)",
			body: `{"projectId":"p","serviceId":"s","runId":"r","statements":["SELECT 1;"],"stream":true}`,
			want: "SELECT 1;",
		},
		{
			name: "multiple statements joined with ;",
			body: `{"projectId":"p","serviceId":"s","runId":"r","statements":["SELECT 1","SELECT 2"]}`,
			want: "SELECT 1; SELECT 2",
		},
		{
			name: "falls back to query field when statements is empty",
			body: `{"projectId":"p","serviceId":"s","runId":"r","query":"SELECT 3","statements":[]}`,
			want: "SELECT 3",
		},
		{
			name: "falls back to query field when statements is omitted",
			body: `{"projectId":"p","serviceId":"s","runId":"r","query":"SELECT 4"}`,
			want: "SELECT 4",
		},
		{
			name: "empty when both fields are blank",
			body: `{"projectId":"p","serviceId":"s","runId":"r"}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req executeQueryRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got := req.SQL(); got != tt.want {
				t.Errorf("SQL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecuteSessionQueryRequest_DecodesEmbedded(t *testing.T) {
	body := `{"projectId":"p","serviceId":"s","runId":"r","sessionId":"sess","statements":["SELECT 1"],"stream":true}`
	var req executeSessionQueryRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.SessionID != "sess" {
		t.Errorf("SessionID = %q, want %q", req.SessionID, "sess")
	}
	if req.RunID != "r" {
		t.Errorf("RunID = %q, want %q", req.RunID, "r")
	}
	if got := req.SQL(); got != "SELECT 1" {
		t.Errorf("SQL() = %q, want %q", got, "SELECT 1")
	}
}
