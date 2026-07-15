package function

import (
	"strings"
	"testing"
)

func TestToolPrefix(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"My DB", "my_db"},
		{"my-db", "my_db"},
		{"mydb", "mydb"},
		{"  Weird -- Name!! ", "weird_name"},
		{"データベース", "db"},
		{"a1 b2", "a1_b2"},
	}
	for _, tt := range tests {
		if got := toolPrefix(tt.name); got != tt.want {
			t.Errorf("toolPrefix(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestDisambiguatePrefix(t *testing.T) {
	taken := map[string]bool{"my_db": true}
	got := disambiguatePrefix("my_db", "abc123xyz789", taken)
	if got != "my_db_abc123" {
		t.Errorf("disambiguatePrefix = %q, want my_db_abc123", got)
	}

	taken[got] = true
	next := disambiguatePrefix("my_db", "abc123xyz789", taken)
	if next != "my_db_abc123x" {
		t.Errorf("disambiguatePrefix = %q, want my_db_abc123x", next)
	}
}

func TestNextPrefix(t *testing.T) {
	taken := map[string]bool{}

	if got := nextPrefix("My DB", "id1", taken); got != "my_db" {
		t.Errorf(`nextPrefix("My DB", ...) = %q, want "my_db"`, got)
	}

	// A second database whose name normalizes to the same prefix is
	// disambiguated with an ID-derived suffix.
	if got := nextPrefix("my-db", "abc123xyz789", taken); got != "my_db_abc123" {
		t.Errorf(`nextPrefix("my-db", ...) = %q, want "my_db_abc123"`, got)
	}

	// A prefix landing in the reserved ghost_* namespace must be escaped
	// outright, not just suffixed — appending a suffix to "ghost" still
	// starts with "ghost_", which is the collision being avoided.
	if got := nextPrefix("Ghost", "id2", taken); got == "ghost" || strings.HasPrefix(got, "ghost_") {
		t.Errorf(`nextPrefix("Ghost", ...) = %q, still lands in the reserved ghost_* namespace`, got)
	}
}
