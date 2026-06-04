package serve

import (
	"encoding/json"
	"testing"
)

func TestExecuteQueryRequestDecodesStatements(t *testing.T) {
	body := `{"projectId":"p","serviceId":"s","runId":"r","statements":["SELECT 1","SELECT 2"],"query":"SELECT 3"}`
	var req executeQueryRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(req.Statements) != 2 || req.Statements[0] != "SELECT 1" || req.Statements[1] != "SELECT 2" {
		t.Errorf("Statements = %v, want [SELECT 1 SELECT 2]", req.Statements)
	}
	if req.Query != "SELECT 3" {
		t.Errorf("Query = %q, want %q", req.Query, "SELECT 3")
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
	if len(req.Statements) != 1 || req.Statements[0] != "SELECT 1" {
		t.Errorf("Statements = %v, want [SELECT 1]", req.Statements)
	}
}
