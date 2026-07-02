package query

import "testing"

func TestDeclaredName(t *testing.T) {
	name, ok := DeclaredQueryName("-- name: CreateAuthor :one\nINSERT INTO authors DEFAULT VALUES;")
	if !ok || name != "CreateAuthor" {
		t.Errorf("DeclaredQueryName = %q, %v; want CreateAuthor, true", name, ok)
	}
	if _, ok := DeclaredQueryName("SELECT 1;"); ok {
		t.Error("expected no directive to be found")
	}
}

func TestValidateQueryBlock(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		query   string
		wantErr bool
	}{
		{
			name:  "valid block",
			tool:  "get_author",
			query: "-- name: get_author :one\n-- Retrieves a single author.\nSELECT id, name FROM authors WHERE id = $1;",
		},
		{
			name:    "invalid tool name",
			tool:    "get author",
			query:   "-- name: get author :one\nSELECT 1;",
			wantErr: true,
		},
		{
			name:    "missing directive",
			tool:    "get_author",
			query:   "SELECT id, name FROM authors WHERE id = $1;",
			wantErr: true,
		},
		{
			name:    "multiple directives",
			tool:    "get_author",
			query:   "-- name: get_author :one\nSELECT 1;\n-- name: extra :one\nSELECT 2;",
			wantErr: true,
		},
		{
			name:    "mismatched name",
			tool:    "get_author",
			query:   "-- name: list_authors :many\nSELECT id, name FROM authors;",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQueryBlock(tt.tool, tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQueryBlock(%q) error = %v, wantErr %v", tt.tool, err, tt.wantErr)
			}
		})
	}
}

func TestJoinQueryBlocks(t *testing.T) {
	queries := []StoredQuery{
		{Name: "get_author", SQL: "\n-- name: get_author :one\nSELECT id, name FROM authors WHERE id = $1;\n\n"},
		{Name: "list_authors", SQL: "-- name: list_authors :many\nSELECT id, name FROM authors ORDER BY name;"},
	}
	got := string(JoinQueryBlocks(queries))
	want := "-- name: get_author :one\nSELECT id, name FROM authors WHERE id = $1;\n" +
		"\n" +
		"-- name: list_authors :many\nSELECT id, name FROM authors ORDER BY name;\n"
	if got != want {
		t.Errorf("JoinQueryBlocks returned:\n%q\nwant:\n%q", got, want)
	}
}
