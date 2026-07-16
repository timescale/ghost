package function

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestInputSchemaRejectsExplicitNullForRequiredArg(t *testing.T) {
	tl := tool{
		Params: []param{
			{Name: "p_customer_id", ArgName: "p_customer_id", Type: typeInfo{Name: "integer"}},
		},
	}
	resolved, err := buildInputSchema(tl).Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := resolved.Validate(map[string]any{"p_customer_id": nil}); err == nil {
		t.Error("expected a validation error for explicit null on a required, non-nullable argument")
	}
	if err := resolved.Validate(map[string]any{"p_customer_id": 1}); err != nil {
		t.Errorf("unexpected validation error for a valid value: %v", err)
	}
	if err := resolved.Validate(map[string]any{"p_customer_id": "not a number"}); err == nil {
		t.Error("expected a validation error for a type mismatch")
	}
	if err := resolved.Validate(map[string]any{}); err == nil {
		t.Error("expected a validation error for a missing required argument")
	}
}

func TestInputSchemaAllowsNullDefault(t *testing.T) {
	tl := tool{
		Params: []param{
			{Name: "p_segment", ArgName: "p_segment", HasDefault: true, NullDefault: true, Type: typeInfo{Name: "text"}},
		},
	}
	resolved, err := buildInputSchema(tl).Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := resolved.Validate(map[string]any{"p_segment": nil}); err != nil {
		t.Errorf("expected null to be allowed for a DEFAULT NULL argument: %v", err)
	}
}
