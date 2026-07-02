package query

import "testing"

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
