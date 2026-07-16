package function

import (
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
)

// newTestManager returns a Manager backed by a real, in-memory MCP server
// (registerServiceTools never dials the database, so a nil pool is fine as
// long as the tools it registers are never actually called).
func newTestManager(prefixTools bool) *Manager {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	return NewManager(nil, server, slog.New(slog.DiscardHandler), prefixTools)
}

func TestRegisterServiceToolsComposesNames(t *testing.T) {
	m := newTestManager(true)
	svc := &service{
		database: api.Database{Id: "db1", Name: "billing"},
		prefix:   "billing",
		tools: []tool{
			{Schema: "public", Name: "get_user", Mode: modeExec},
			{Schema: "reporting", Name: "get_order", Mode: modeExec},
		},
	}

	m.registerServiceTools(svc)

	// Schema plays no part in the name at all — a "reporting" schema
	// function is named exactly like a "public" one would be.
	want := []string{"billing__get_user", "billing__get_order"}
	if !slices.Equal(svc.toolNames, want) {
		t.Errorf("toolNames = %v, want %v", svc.toolNames, want)
	}
	for _, name := range want {
		if m.toolNames[name] != "db1" {
			t.Errorf("toolNames[%q] = %q, want \"db1\"", name, m.toolNames[name])
		}
	}
}

func TestRegisterServiceToolsOmitsPrefixInServeMode(t *testing.T) {
	m := newTestManager(false)
	svc := &service{
		database: api.Database{Id: "db1", Name: "billing"},
		// prefix left empty, as Load leaves it when prefixTools is false.
		tools: []tool{
			{Schema: "public", Name: "get_user", Mode: modeExec},
		},
	}

	m.registerServiceTools(svc)

	want := []string{"get_user"}
	if !slices.Equal(svc.toolNames, want) {
		t.Errorf("toolNames = %v, want %v", svc.toolNames, want)
	}
}

func TestRegisterServiceToolsDedupesSameNameAcrossSchemas(t *testing.T) {
	m := newTestManager(true)
	// Same function name in three different schemas: now that schema isn't
	// part of the tool name, this is the common source of collisions, not
	// an edge case. Order here is the processing order (as introspect's own
	// ORDER BY would produce for a real database) — the first gets the bare
	// name, the rest get numeric suffixes, never dropped.
	svc := &service{
		database: api.Database{Id: "db1", Name: "billing"},
		prefix:   "billing",
		tools: []tool{
			{Schema: "public", Name: "get_user", Mode: modeExec},
			{Schema: "reporting", Name: "get_user", Mode: modeExec},
			{Schema: "admin", Name: "get_user", Mode: modeExec},
		},
	}

	m.registerServiceTools(svc)

	want := []string{"billing__get_user", "billing__get_user_2", "billing__get_user_3"}
	if !slices.Equal(svc.toolNames, want) {
		t.Errorf("toolNames = %v, want %v", svc.toolNames, want)
	}
}

func TestRegisterServiceToolsDedupesAcrossDatabases(t *testing.T) {
	m := newTestManager(true)
	// Two different, differently-named databases whose names normalize to
	// the identical prefix ("My!DB" and "My@DB" both collapse their illegal
	// character to "_"), each defining the same function name. Simulates
	// LoadAll's deterministic sequential registration pass: both are
	// registered in a fixed order (here, A before B).
	svcA := &service{
		database: api.Database{Id: "dbA", Name: "My!DB"},
		prefix:   normalizeToolNameSegment("My!DB", "db"),
		tools:    []tool{{Schema: "public", Name: "get_user", Mode: modeExec}},
	}
	svcB := &service{
		database: api.Database{Id: "dbB", Name: "My@DB"},
		prefix:   normalizeToolNameSegment("My@DB", "db"),
		tools:    []tool{{Schema: "public", Name: "get_user", Mode: modeExec}},
	}

	m.registerServiceTools(svcA)
	m.registerServiceTools(svcB)

	if want := []string{"My_DB__get_user"}; !slices.Equal(svcA.toolNames, want) {
		t.Errorf("svcA toolNames = %v, want %v", svcA.toolNames, want)
	}
	if want := []string{"My_DB__get_user_2"}; !slices.Equal(svcB.toolNames, want) {
		t.Errorf("svcB toolNames = %v, want %v (disambiguated, not dropped, despite colliding with svcA's)", svcB.toolNames, want)
	}
}

func TestRegisterServiceToolsNormalizesIllegalCharacters(t *testing.T) {
	m := newTestManager(false)
	svc := &service{
		database: api.Database{Id: "db1", Name: "billing"},
		tools: []tool{
			{Schema: "public", Name: "short_name", Mode: modeExec},
			{Schema: "public", Name: "get customer", Mode: modeExec}, // space -> underscore
			{Schema: "public", Name: "get_tracks_🎵", Mode: modeExec}, // trailing emoji stripped
		},
	}

	m.registerServiceTools(svc)

	want := []string{"short_name", "get_customer", "get_tracks_"}
	if !slices.Equal(svc.toolNames, want) {
		t.Errorf("toolNames = %v, want %v (illegal characters should be normalized, not dropped)", svc.toolNames, want)
	}
}

func TestNormalizeToolNameSegment(t *testing.T) {
	tests := []struct {
		name     string
		fallback string
		want     string
	}{
		{"get_user", "tool", "get_user"},
		{"My-Table", "tool", "My-Table"},
		{"get customer", "tool", "get_customer"},
		{"get  customer", "tool", "get_customer"}, // a run of illegal chars collapses to one "_"
		{"get_tracks_🎵", "tool", "get_tracks_"},
		{"🎵🎶", "tool", "tool"}, // nothing legal survives: falls back
		{"", "tool", "tool"},
		{"🎵🎶", "db", "db"}, // fallback depends on the segment kind, not hardcoded
		{"", "db", "db"},
		{"!!!leading", "tool", "leading"},
		{"trailing!!!", "tool", "trailing"},
	}
	for _, tt := range tests {
		if got := normalizeToolNameSegment(tt.name, tt.fallback); got != tt.want {
			t.Errorf("normalizeToolNameSegment(%q, %q) = %q, want %q", tt.name, tt.fallback, got, tt.want)
		}
	}
}

func TestRegisterServiceToolsTruncatesOverLengthName(t *testing.T) {
	m := newTestManager(false)
	longName := strings.Repeat("x", maxToolNameLength+50)
	svc := &service{
		database: api.Database{Id: "db1", Name: "billing"},
		tools: []tool{
			{Schema: "public", Name: "short_name", Mode: modeExec},
			{Schema: "public", Name: longName, Mode: modeExec},
		},
	}

	m.registerServiceTools(svc)

	if len(svc.toolNames) != 2 {
		t.Fatalf("toolNames = %v, want 2 entries (over-length name should be truncated, not dropped)", svc.toolNames)
	}
	truncated := svc.toolNames[1]
	if want := strings.Repeat("x", maxToolNameLength); truncated != want {
		t.Errorf("truncated tool name = %q, want %q", truncated, want)
	}
}

func TestRegisterServiceToolsDedupesAfterTruncation(t *testing.T) {
	m := newTestManager(false)
	// Two names identical for the first maxToolNameLength characters,
	// differing only in a tail that truncation alone would cut away —
	// truncating both would otherwise make them collide.
	base := strings.Repeat("x", maxToolNameLength)
	svc := &service{
		database: api.Database{Id: "db1", Name: "billing"},
		tools: []tool{
			{Schema: "public", Name: base + "aaaa", Mode: modeExec},
			{Schema: "public", Name: base + "bbbb", Mode: modeExec},
		},
	}

	m.registerServiceTools(svc)

	if len(svc.toolNames) != 2 || svc.toolNames[0] == svc.toolNames[1] {
		t.Fatalf("toolNames = %v, want 2 distinct entries even though both truncate to the same base", svc.toolNames)
	}
	for _, name := range svc.toolNames {
		if len(name) > maxToolNameLength {
			t.Errorf("tool name %q exceeds maxToolNameLength (%d)", name, maxToolNameLength)
		}
	}
	if want := "_2"; !strings.HasSuffix(svc.toolNames[1], want) {
		t.Errorf("second tool name = %q, want a %q suffix", svc.toolNames[1], want)
	}
}

func TestRegisterServiceToolsReloadIsStable(t *testing.T) {
	m := newTestManager(true)
	tools := []tool{
		{Schema: "public", Name: "get_user", Mode: modeExec},
		{Schema: "reporting", Name: "get_user", Mode: modeExec},
		{Schema: "public", Name: "get_order", Mode: modeExec},
	}
	svc := &service{
		database: api.Database{Id: "db1", Name: "billing"},
		prefix:   "billing",
		tools:    slices.Clone(tools),
	}

	m.registerServiceTools(svc)
	first := slices.Clone(svc.toolNames)

	// Simulate a ghost_mcp_tool_refresh: re-introspection returns the same
	// functions, in the same (introspect-guaranteed) order, and Load's
	// "already loaded" branch swaps the registered tools via
	// swapServiceTools.
	m.swapServiceTools(svc, slices.Clone(tools))
	second := slices.Clone(svc.toolNames)

	if !slices.Equal(first, second) {
		t.Errorf("reload produced different tool names: first=%v, second=%v", first, second)
	}
}
