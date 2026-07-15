package function

import "testing"

func TestParseMarkerComment(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		wantDesc string
		wantOK   bool
	}{
		{
			name:     "bare marker with description",
			comment:  "@mcp\nReturns unpaid invoices for a customer.",
			wantDesc: "Returns unpaid invoices for a customer.",
			wantOK:   true,
		},
		{
			name:     "marker only",
			comment:  "@mcp",
			wantDesc: "",
			wantOK:   true,
		},
		{
			name:     "leading whitespace",
			comment:  "\n  @mcp\nDescription here.",
			wantDesc: "Description here.",
			wantOK:   true,
		},
		{
			name:     "multi-line description",
			comment:  "@mcp\nLine one.\nLine two.",
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
			comment: "Some description.\n@mcp",
			wantOK:  false,
		},
		{
			name:    "marker with trailing text on same line",
			comment: "@mcp this is not the syntax",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, ok := parseMarkerComment(tt.comment)
			if ok != tt.wantOK {
				t.Fatalf("parseMarkerComment(%q) ok = %v, want %v", tt.comment, ok, tt.wantOK)
			}
			if desc != tt.wantDesc {
				t.Errorf("parseMarkerComment(%q) desc = %q, want %q", tt.comment, desc, tt.wantDesc)
			}
		})
	}
}

func TestBuildToolRejectsNonFunctionKinds(t *testing.T) {
	// The kind check runs before any type resolution, so no resolver is
	// needed.
	for _, kind := range []string{"p", "a", "w"} {
		if _, err := buildTool(nil, functionRow{Kind: kind}); err == nil {
			t.Errorf("buildTool(kind %q) succeeded, want error", kind)
		}
	}
}

func TestIsNullDefault(t *testing.T) {
	tests := []struct {
		def  string
		want bool
	}{
		{"NULL", true},
		{"NULL::integer", true},
		{"NULL::character varying", true},
		{"5", false},
		{"'NULL'::text", false},
		{"NULLIF(1, 1)", false},
		{"''::text", false},
	}
	for _, tt := range tests {
		if got := isNullDefault(tt.def); got != tt.want {
			t.Errorf("isNullDefault(%q) = %v, want %v", tt.def, got, tt.want)
		}
	}
}

func TestInputParamsFallbackNameAvoidsCollision(t *testing.T) {
	// f(integer, param_1 integer): the first argument is unnamed, so it
	// would naively fall back to "param_1" — colliding with the second
	// argument's own explicit name.
	resolver := &typeResolver{types: map[int64]typeRow{
		23: {OID: 23, Name: "integer", TypeType: "b"},
	}}
	row := functionRow{
		ArgTypes: []int64{23, 23},
		ArgNames: []string{"", "param_1"},
	}

	params, err := inputParams(resolver, row)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 2 {
		t.Fatalf("len(params) = %d, want 2", len(params))
	}
	if params[0].Name == params[1].Name {
		t.Fatalf("both params got the name %q: fallback name collided with the explicit argument name", params[0].Name)
	}
	if params[1].Name != "param_1" {
		t.Errorf(`params[1].Name = %q, want "param_1" (the function's own explicit name)`, params[1].Name)
	}
	if params[0].Name != "param_2" {
		t.Errorf(`params[0].Name = %q, want "param_2" (the next available fallback)`, params[0].Name)
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
