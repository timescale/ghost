package function

import "testing"

func TestBuildCall(t *testing.T) {
	tl := tool{
		Schema:    "public",
		Name:      "get_pending_invoices",
		Mode:      modeMany,
		NamedArgs: true,
		Params: []param{
			{Name: "p_customer_id", ArgName: "p_customer_id", Type: typeInfo{Name: "integer"}},
			{Name: "p_limit", ArgName: "p_limit", HasDefault: true, Type: typeInfo{Name: "integer"}},
			{Name: "p_segment", ArgName: "p_segment", HasDefault: true, Type: typeInfo{Name: "text"}},
		},
	}

	t.Run("all provided uses positional notation", func(t *testing.T) {
		sql, args, err := buildCall(tl, map[string]any{"p_customer_id": 1, "p_limit": 5, "p_segment": "b"})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."get_pending_invoices"($1::integer, $2::integer, $3::text)`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
		if len(args) != 3 {
			t.Errorf("len(args) = %d, want 3", len(args))
		}
	})

	t.Run("omitted default uses named notation", func(t *testing.T) {
		sql, args, err := buildCall(tl, map[string]any{"p_customer_id": 1, "p_segment": "b"})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."get_pending_invoices"("p_customer_id" => $1::integer, "p_segment" => $2::text)`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
		if len(args) != 2 {
			t.Errorf("len(args) = %d, want 2", len(args))
		}
	})

	t.Run("missing required parameter", func(t *testing.T) {
		if _, _, err := buildCall(tl, map[string]any{"p_limit": 5}); err == nil {
			t.Error("expected error for missing required parameter")
		}
	})

	t.Run("unnamed args allow trailing omission only", func(t *testing.T) {
		unnamed := tool{
			Schema: "public",
			Name:   "f",
			Mode:   modeOne,
			Params: []param{
				{Name: "param_1", Type: typeInfo{Name: "integer"}},
				{Name: "param_2", HasDefault: true, Type: typeInfo{Name: "integer"}},
				{Name: "param_3", HasDefault: true, Type: typeInfo{Name: "integer"}},
			},
		}
		sql, _, err := buildCall(unnamed, map[string]any{"param_1": 1, "param_2": 2})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."f"($1::integer, $2::integer)`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
		if _, _, err := buildCall(unnamed, map[string]any{"param_1": 1, "param_3": 3}); err == nil {
			t.Error("expected error for non-trailing omission with unnamed arguments")
		}
	})

	t.Run("array argument casts to the element type with a [] suffix", func(t *testing.T) {
		arrayTool := tool{
			Schema: "public",
			Name:   "f",
			Mode:   modeOne,
			Params: []param{
				{Name: "ids", ArgName: "ids", Type: typeInfo{Name: "integer", IsArray: true}},
			},
		}
		sql, _, err := buildCall(arrayTool, map[string]any{"ids": []any{1, 2}})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."f"($1::integer[])`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
	})
}
