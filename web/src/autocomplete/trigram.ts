// pg_trgm-style trigram utilities, used to fuzzy-rank schema-object names for
// autocomplete. This mirrors the matching PostgreSQL's pg_trgm extension does
// server-side in popsql's SchemaService, so suggestions rank the same way here
// (where we search an in-memory index instead of hitting Postgres).
//
// PostgreSQL's pg_trgm lowercases the input, splits it into maximal
// alphanumeric words, pads each word with two leading spaces and one trailing
// space, then takes every 3-character window. e.g. show_trgm('word') yields
// {"  w", " wo", "wor", "ord", "rd "}.

const ALNUM = /[a-z0-9]+/g;

// trigrams returns the pg_trgm trigram set for a string (lowercased, word-split,
// space-padded). Non-alphanumeric characters are treated as word boundaries.
export function trigrams(input: string): Set<string> {
  const set = new Set<string>();
  const words = input.toLowerCase().match(ALNUM);
  if (!words) return set;
  for (const word of words) {
    const padded = `  ${word} `;
    for (let i = 0; i + 3 <= padded.length; i++) {
      set.add(padded.slice(i, i + 3));
    }
  }
  return set;
}

// wordSimilarity approximates PostgreSQL's word_similarity(query, name): the
// fraction of the query's trigrams that also appear in the name. It is
// asymmetric (normalized by the query, not the union), which is what makes it
// useful for autocomplete — a short query fully contained in a long name scores
// high, so typing "usr" still surfaces "user_sessions". Returns a value in
// [0, 1]; 0 when the query has no trigrams (e.g. empty or all-punctuation).
export function wordSimilarity(
  queryTrigrams: Set<string>,
  nameTrigrams: Set<string>,
): number {
  if (queryTrigrams.size === 0) return 0;
  let shared = 0;
  for (const trigram of queryTrigrams) {
    if (nameTrigrams.has(trigram)) shared += 1;
  }
  return shared / queryTrigrams.size;
}

// longestAlphanumericRun returns the length of the longest run of alphanumeric
// characters in the query. This is the same signal popsql uses to pick a
// matching strategy: pg_trgm can only use a trigram index for substring matches
// once the query has at least 3 alphanumeric characters, so shorter queries
// fall back to prefix matching. Ported from popsql's longestAlphanumericSubstring.
export function longestAlphanumericRun(query: string): number {
  let max = 0;
  let current = 0;
  for (let i = 0; i < query.length; i++) {
    const code = query.charCodeAt(i);
    const isAlnum =
      (code > 47 && code < 58) || // 0-9
      (code > 64 && code < 91) || // A-Z
      (code > 96 && code < 123); // a-z
    if (isAlnum) {
      current += 1;
      if (current > max) max = current;
    } else {
      current = 0;
    }
  }
  return max;
}
