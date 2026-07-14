package function

import (
	"regexp"
	"strings"
)

// On the authoring server, generated tool names are prefixed with the
// snake_cased database name to avoid collisions between services — a
// function named `whatever` on a database named "My DB" becomes the tool
// `my_db_whatever`. (The consumer serving mode exposes a single database's
// tools and nothing else, so it skips the prefix.) Some normalization of the
// database name is unavoidable: model APIs restrict tool names to
// [a-zA-Z0-9_-], so spaces and other characters can't survive into the tool
// name.

// toolNamePattern restricts function names to the characters model APIs
// accept in tool names. A quoted Postgres identifier can contain anything,
// so @mcp functions whose names don't fit are skipped with a warning.
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
