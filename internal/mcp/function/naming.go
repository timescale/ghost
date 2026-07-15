package function

import (
	"regexp"
	"strings"
)

// On the authoring server, generated tool names are prefixed with the
// snake_cased database name to avoid collisions between services — a
// function named `whatever` on a database named "My DB" becomes the tool
// `my_db_whatever`. (The consumer serving mode exposes a single database's
// tools and nothing else, so it skips the prefix.) A function's own schema
// becomes part of the name too, unless it's "public", so two @mcp functions
// of the same name in different schemas of one database don't collide. Some
// normalization of the database name is unavoidable: model APIs restrict
// tool names to [a-zA-Z0-9_-], so spaces and other characters can't survive
// into the tool name.

// toolNamePattern restricts function and schema names to the characters
// model APIs accept in tool names. A quoted Postgres identifier can contain
// anything, so an @mcp function (or its schema) that doesn't fit is skipped
// with a warning.
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// toolPrefix normalizes a database name into a tool-name prefix: lowercased,
// with every run of characters outside [a-z0-9] collapsed into a single
// underscore. A name with no usable characters falls back to "db".
func toolPrefix(name string) string {
	var b strings.Builder
	pendingSep := false
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if pendingSep && b.Len() > 0 {
				b.WriteByte('_')
			}
			pendingSep = false
			b.WriteRune(r)
		default:
			pendingSep = true
		}
	}
	if b.Len() == 0 {
		return "db"
	}
	return b.String()
}

// disambiguatePrefix appends a short suffix derived from the database ID to a
// conflicting prefix, extending the suffix until the result is unique among
// the already-taken prefixes.
func disambiguatePrefix(prefix, databaseID string, taken map[string]bool) string {
	id := toolPrefix(databaseID)
	for n := min(6, len(id)); n <= len(id); n++ {
		candidate := prefix + "_" + id[:n]
		if !taken[candidate] {
			return candidate
		}
	}
	return prefix + "_" + id
}

// nextPrefix computes the tool-name prefix for a database, given the
// prefixes already taken by other databases — which it updates, so
// repeated calls over a database list assign every one a unique prefix.
// Database names are unique within a space, so a collision only arises
// when two names differ solely by case or separator style once normalized
// by toolPrefix. A prefix landing in the built-in ghost_* tool namespace is
// treated as a collision too, but can't be disambiguated by appending a
// suffix (the result would still start with "ghost_"), so it's rewritten
// outright first.
func nextPrefix(name, databaseID string, taken map[string]bool) string {
	prefix := toolPrefix(name)
	if prefix == "ghost" || strings.HasPrefix(prefix, "ghost_") {
		prefix = "db_" + prefix
	}
	if taken[prefix] {
		prefix = disambiguatePrefix(prefix, databaseID, taken)
	}
	taken[prefix] = true
	return prefix
}
