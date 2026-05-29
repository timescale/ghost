// countStatements walks `sql` once and counts top-level statements,
// approximating what pgx's simple text protocol will execute when the widget
// sends the editor contents joined by `; `. Comments, single-quoted strings
// (with `''` escapes), double-quoted identifiers, and dollar-quoted blocks
// are skipped so semicolons inside them don't inflate the count.
//
// This is an approximation, not a real parser — Unicode identifier rules,
// nested block comments, and a few other corners aren't handled. Good enough
// for displaying "Executed N statements" in the toolbar; the server-side
// `executedStatements` field on the success line is the authoritative count.
export function countStatements(sql: string): number {
  let count = 0;
  let nonWhitespaceInStmt = 0;
  let i = 0;
  const len = sql.length;

  while (i < len) {
    const c = sql[i];
    const next = sql[i + 1];

    // -- line comment
    if (c === '-' && next === '-') {
      i += 2;
      while (i < len && sql[i] !== '\n') i++;
      continue;
    }
    // /* block comment */ (not nested)
    if (c === '/' && next === '*') {
      i += 2;
      while (i < len && !(sql[i] === '*' && sql[i + 1] === '/')) i++;
      if (i < len) i += 2;
      continue;
    }
    // single-quoted string '...' with '' escape
    if (c === "'") {
      nonWhitespaceInStmt++;
      i++;
      while (i < len) {
        if (sql[i] === "'") {
          if (sql[i + 1] === "'") {
            i += 2;
            continue;
          }
          i++;
          break;
        }
        i++;
      }
      continue;
    }
    // double-quoted identifier "..." with "" escape
    if (c === '"') {
      nonWhitespaceInStmt++;
      i++;
      while (i < len) {
        if (sql[i] === '"') {
          if (sql[i + 1] === '"') {
            i += 2;
            continue;
          }
          i++;
          break;
        }
        i++;
      }
      continue;
    }
    // dollar-quoted: $tag$ ... $tag$ (tag may be empty)
    if (c === '$') {
      const tagMatch = /^\$([A-Za-z_][A-Za-z0-9_]*)?\$/.exec(sql.slice(i));
      if (tagMatch) {
        const tag = tagMatch[0];
        nonWhitespaceInStmt++;
        i += tag.length;
        const endIdx = sql.indexOf(tag, i);
        if (endIdx === -1) {
          i = len;
        } else {
          i = endIdx + tag.length;
        }
        continue;
      }
    }
    // statement terminator
    if (c === ';') {
      if (nonWhitespaceInStmt > 0) count++;
      nonWhitespaceInStmt = 0;
      i++;
      continue;
    }
    if (c !== undefined && /\S/.test(c)) nonWhitespaceInStmt++;
    i++;
  }

  if (nonWhitespaceInStmt > 0) count++;
  return count;
}
