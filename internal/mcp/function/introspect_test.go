package function

import "testing"

func TestParseAPIComment(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		wantDesc string
		wantOK   bool
	}{
		{
			name:     "bare marker with description",
			comment:  "@api\nReturns unpaid invoices for a customer.",
			wantDesc: "Returns unpaid invoices for a customer.",
			wantOK:   true,
		},
		{
			name:     "marker only",
			comment:  "@api",
			wantDesc: "",
			wantOK:   true,
		},
		{
			name:     "leading whitespace",
			comment:  "\n  @api\nDescription here.",
			wantDesc: "Description here.",
			wantOK:   true,
		},
		{
			name:     "group list accepted and ignored",
			comment:  "@api(customer_service, accounts_receivable)\nReturns the customer profile.",
			wantDesc: "Returns the customer profile.",
			wantOK:   true,
		},
		{
			name:     "multi-line description",
			comment:  "@api\nLine one.\nLine two.",
			wantDesc: "Line one.\nLine two.",
			wantOK:   true,
		},
		{
			name:    "no marker",
			comment: "Just a regular function comment.",
			wantOK:  false,
		},
		{
			name:    "marker not on first line",
			comment: "Some description.\n@api",
			wantOK:  false,
		},
		{
			name:    "marker with trailing text on same line",
			comment: "@api this is not the syntax",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, ok := parseAPIComment(tt.comment)
			if ok != tt.wantOK {
				t.Fatalf("parseAPIComment(%q) ok = %v, want %v", tt.comment, ok, tt.wantOK)
			}
			if desc != tt.wantDesc {
				t.Errorf("parseAPIComment(%q) desc = %q, want %q", tt.comment, desc, tt.wantDesc)
			}
		})
	}
}

func TestBuildCall(t *testing.T) {
	tool := Tool{
		Schema: "public",
		Name:   "get_pending_invoices",
		Mode:   ModeMany,
		Named:  true,
		Params: []Param{
			{Name: "p_customer_id", ArgName: "p_customer_id", Type: TypeInfo{Name: "integer"}},
			{Name: "p_limit", ArgName: "p_limit", HasDefault: true, Type: TypeInfo{Name: "integer"}},
			{Name: "p_segment", ArgName: "p_segment", HasDefault: true, Type: TypeInfo{Name: "text"}},
		},
	}

	t.Run("all provided uses positional notation", func(t *testing.T) {
		sql, args, err := buildCall(tool, map[string]any{"p_customer_id": 1, "p_limit": 5, "p_segment": "b"})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."get_pending_invoices"($1, $2, $3)`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
		if len(args) != 3 {
			t.Errorf("len(args) = %d, want 3", len(args))
		}
	})

	t.Run("omitted default uses named notation", func(t *testing.T) {
		sql, args, err := buildCall(tool, map[string]any{"p_customer_id": 1, "p_segment": "b"})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."get_pending_invoices"("p_customer_id" => $1, "p_segment" => $2)`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
		if len(args) != 2 {
			t.Errorf("len(args) = %d, want 2", len(args))
		}
	})

	t.Run("missing required parameter", func(t *testing.T) {
		if _, _, err := buildCall(tool, map[string]any{"p_limit": 5}); err == nil {
			t.Error("expected error for missing required parameter")
		}
	})

	t.Run("unnamed args allow trailing omission only", func(t *testing.T) {
		unnamed := Tool{
			Schema: "public",
			Name:   "f",
			Mode:   ModeOne,
			Params: []Param{
				{Name: "param_1", Type: TypeInfo{Name: "integer"}},
				{Name: "param_2", HasDefault: true, Type: TypeInfo{Name: "integer"}},
				{Name: "param_3", HasDefault: true, Type: TypeInfo{Name: "integer"}},
			},
		}
		sql, _, err := buildCall(unnamed, map[string]any{"param_1": 1, "param_2": 2})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."f"($1, $2)`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
		if _, _, err := buildCall(unnamed, map[string]any{"param_1": 1, "param_3": 3}); err == nil {
			t.Error("expected error for non-trailing omission with unnamed arguments")
		}
	})
}
