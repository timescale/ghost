package query

// The functions below operate on sqlc query blocks. A block starts at a
// `-- name: <Name> :<cmd>` directive and extends to the next directive (or end
// of content), encompassing any doc comments and the SQL itself. Each stored
// query holds exactly one block; the builder joins them into a single query
// file for sqlc.

import (
	"fmt"
	"regexp"
	"strings"
)

// nameDirective matches the sqlc query-name directive at the start of a line,
// capturing the query name (the token before the `:cmd` suffix).
var nameDirective = regexp.MustCompile(`(?m)^[ \t]*--[ \t]*name:[ \t]*(\S+)`)

// toolNamePattern restricts query (and therefore tool) names to the characters
// model APIs accept in tool names. The database-name prefix joined onto the
// front is normalized to the same alphabet (see toolPrefix).
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// block describes the location of a single query within file content.
type block struct {
	name  string
	start int // byte offset of the directive line
	end   int // byte offset just past the block (start of next block or len)
}

// parseBlocks locates every query block in the given content.
func parseBlocks(content []byte) []block {
	matches := nameDirective.FindAllSubmatchIndex(content, -1)
	blocks := make([]block, 0, len(matches))
	for i, m := range matches {
		end := len(content)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		blocks = append(blocks, block{
			name:  string(content[m[2]:m[3]]),
			start: m[0],
			end:   end,
		})
	}
	return blocks
}

// DeclaredQueryName returns the query name declared by the first sqlc directive
// in block. The second return value reports whether a directive was found.
func DeclaredQueryName(block string) (string, bool) {
	m := nameDirective.FindStringSubmatch(block)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// ValidateQueryBlock checks that query is a single sqlc query block whose
// directive declares the given name, so the resulting tool registers under the
// requested name and a stored query cannot smuggle in additional tools.
func ValidateQueryBlock(name, query string) error {
	if !toolNamePattern.MatchString(name) {
		return fmt.Errorf("invalid query tool name %q: only letters, digits, underscores, and hyphens are allowed", name)
	}
	blocks := parseBlocks([]byte(query))
	if len(blocks) == 0 {
		return fmt.Errorf("query must include a sqlc directive like '-- name: %s :one'", name)
	}
	if len(blocks) > 1 {
		return fmt.Errorf("query must contain exactly one '-- name:' directive, found %d", len(blocks))
	}
	if declared := blocks[0].name; declared != name {
		return fmt.Errorf("the query's '-- name:' directive (%q) must match the tool name (%q)", declared, name)
	}
	return nil
}

// JoinQueryBlocks concatenates stored query blocks into a single sqlc query
// file, with each block normalized to end in exactly one newline and separated
// by a blank line.
func JoinQueryBlocks(queries []StoredQuery) []byte {
	blocks := make([]string, len(queries))
	for i, q := range queries {
		blocks[i] = normalize(q.SQL)
	}
	return []byte(strings.Join(blocks, "\n"))
}

// normalize trims surrounding blank lines from a block and ensures it ends with
// exactly one newline.
func normalize(block string) string {
	return strings.Trim(block, "\n") + "\n"
}
