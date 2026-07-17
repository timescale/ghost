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

func TestInputParamsVariadic(t *testing.T) {
	// f(a text, VARIADIC vals integer[]): the variadic argument's declared
	// type is the array type (integer[]), so it resolves to an array parameter
	// and is flagged Variadic.
	resolver := &typeResolver{types: map[int64]typeRow{
		25:   {OID: 25, Name: "text", TypeType: "b"},
		1007: {OID: 1007, Name: "integer[]", TypeType: "b", Category: "A", Elem: 23},
		23:   {OID: 23, Name: "integer", TypeType: "b"},
	}}
	row := functionRow{
		ArgTypes: []int64{25, 1007},
		ArgModes: []string{"i", "v"},
		ArgNames: []string{"a", "vals"},
	}

	params, err := inputParams(resolver, row)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 2 {
		t.Fatalf("len(params) = %d, want 2", len(params))
	}
	if params[0].Variadic {
		t.Error("params[0] (a) should not be flagged variadic")
	}
	if !params[1].Variadic {
		t.Error("params[1] (vals) should be flagged variadic")
	}
	if !params[1].Type.IsArray || params[1].Type.Name != "integer" {
		t.Errorf("params[1].Type = %+v, want an integer array", params[1].Type)
	}
}

func TestInputParamsVariadicAnyRejected(t *testing.T) {
	// VARIADIC "any" (2276) is a pseudo-type; a value of it can't cross the
	// tool boundary, so the function is rejected (and skipped by buildTool).
	resolver := &typeResolver{types: map[int64]typeRow{
		2276: {OID: 2276, Name: "\"any\"", TypeType: "p", Category: "P"},
	}}
	row := functionRow{
		ArgTypes: []int64{2276},
		ArgModes: []string{"v"},
		ArgNames: []string{"args"},
	}
	if _, err := inputParams(resolver, row); err == nil {
		t.Error(`expected error for VARIADIC "any" argument`)
	}
}
