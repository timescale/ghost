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

func TestBuildCallVariadic(t *testing.T) {
	// f(a text, VARIADIC vals integer[]): the variadic argument is a required
	// array parameter passed with the VARIADIC keyword.
	tl := tool{
		Schema:    "public",
		Name:      "f",
		Mode:      modeOne,
		NamedArgs: true,
		Params: []param{
			{Name: "a", ArgName: "a", Type: typeInfo{Name: "text"}},
			{Name: "vals", ArgName: "vals", Variadic: true, Type: typeInfo{Name: "integer", IsArray: true}},
		},
	}

	t.Run("variadic array passed with the VARIADIC keyword", func(t *testing.T) {
		sql, args, err := buildCall(tl, map[string]any{"a": "x", "vals": []any{1, 2}})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."f"($1::text, VARIADIC $2::integer[])`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
		if len(args) != 2 {
			t.Errorf("len(args) = %d, want 2", len(args))
		}
	})

	t.Run("empty variadic array is passed through, not omitted", func(t *testing.T) {
		// An empty array is a real value the VARIADIC keyword forwards, which
		// is the only way to pass a variadic function zero elements.
		sql, _, err := buildCall(tl, map[string]any{"a": "x", "vals": []any{}})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."f"($1::text, VARIADIC $2::integer[])`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
	})

	t.Run("variadic function is positional-only", func(t *testing.T) {
		// f(a text, b integer DEFAULT 5, VARIADIC vals integer[] DEFAULT '{}'):
		// even with named arguments, PostgreSQL can't skip a middle default
		// under named notation for a variadic call, so providing a + vals while
		// omitting b is rejected rather than emitted as an invalid call.
		tl := tool{
			Schema:    "public",
			Name:      "f",
			Mode:      modeOne,
			NamedArgs: true,
			Params: []param{
				{Name: "a", ArgName: "a", Type: typeInfo{Name: "text"}},
				{Name: "b", ArgName: "b", HasDefault: true, Type: typeInfo{Name: "integer"}},
				{Name: "vals", ArgName: "vals", HasDefault: true, Variadic: true, Type: typeInfo{Name: "integer", IsArray: true}},
			},
		}
		if _, _, err := buildCall(tl, map[string]any{"a": "x", "vals": []any{1}}); err == nil {
			t.Error("expected error: a variadic function can't skip a middle default")
		}

		// Trailing omissions (omit b and the variadic) are allowed and use
		// positional notation.
		sql, _, err := buildCall(tl, map[string]any{"a": "x"})
		if err != nil {
			t.Fatal(err)
		}
		want := `SELECT * FROM "public"."f"($1::text)`
		if sql != want {
			t.Errorf("sql = %q, want %q", sql, want)
		}
	})
}
