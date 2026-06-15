import type {
  SchemaFetchResponse,
  SuggestionRequest,
} from '@timescale/popsql-query-widget-cdn';

import { longestAlphanumericRun, trigrams, wordSimilarity } from './trigram';

// Maximum results returned per request, matching popsql's SchemaService limit.
const RESULT_LIMIT = 100;

// Minimum word_similarity for a fuzzy (non-substring) match, matching the
// pg_trgm word_similarity_threshold popsql relies on.
const SIMILARITY_THRESHOLD = 0.5;

// An indexed object is a flattened schema entry plus its precomputed lowercase
// name and trigram set, so per-keystroke search avoids recomputing them.
interface IndexedObject {
  response: SchemaFetchResponse;
  nameLower: string;
  trigrams: Set<string>;
}

export interface SchemaIndex {
  objects: IndexedObject[];
}

// buildIndex precomputes the lowercase name and trigram set for each flattened
// object once, when the schema is (re)loaded.
export function buildIndex(responses: SchemaFetchResponse[]): SchemaIndex {
  const objects = responses.map((response) => ({
    response,
    nameLower: response.name.toLowerCase(),
    trigrams: trigrams(response.name),
  }));
  return { objects };
}

// matchesScope returns whether an object satisfies a request's database /
// schema / table / type filters. All comparisons are case-insensitive; an
// absent filter field matches anything.
function matchesScope(obj: IndexedObject, request: SuggestionRequest): boolean {
  const { response } = obj;
  if (request.type && !request.type.includes(response.type as never)) {
    return false;
  }
  if (
    request.database &&
    response.database?.toLowerCase() !== request.database.toLowerCase()
  ) {
    return false;
  }
  if (
    request.schema &&
    response.schema?.toLowerCase() !== request.schema.toLowerCase()
  ) {
    return false;
  }
  if (
    request.table &&
    response.table?.toLowerCase() !== request.table.toLowerCase()
  ) {
    return false;
  }
  return true;
}

// scoreMatch decides whether a name matches the query and returns its score,
// or null if it doesn't match. The tiers mirror popsql's SchemaService, which
// picks a strategy based on the longest alphanumeric run in the query because
// pg_trgm can only index substring matches at 3+ characters:
//
//   - empty query        → everything matches (listing); score 0
//   - run <= 2 chars      → prefix match, or word_similarity >= threshold
//   - run 3-4 chars       → substring match (ILIKE '%q%' in popsql)
//   - run >= 5 chars      → substring match OR word_similarity >= threshold
//
// In every non-empty tier the score is word_similarity, so results rank by
// fuzzy closeness regardless of which tier admitted them.
function scoreMatch(
  obj: IndexedObject,
  queryLower: string,
  queryTrigrams: Set<string>,
  runLength: number,
): number | null {
  if (queryLower === '') return 0;

  const similarity = wordSimilarity(queryTrigrams, obj.trigrams);
  const contains = obj.nameLower.includes(queryLower);

  if (runLength <= 2) {
    if (
      obj.nameLower.startsWith(queryLower) ||
      similarity >= SIMILARITY_THRESHOLD
    ) {
      return similarity;
    }
    return null;
  }

  if (runLength <= 4) {
    return contains ? similarity : null;
  }

  if (contains || similarity >= SIMILARITY_THRESHOLD) {
    return similarity;
  }
  return null;
}

// searchIndex runs each request against the index and returns matching objects
// (with their per-search score filled in), ranked by score descending then name
// ascending, capped at RESULT_LIMIT per request and flattened across requests —
// matching how popsql issues one query per SuggestionRequest and concatenates.
export function searchIndex(
  index: SchemaIndex,
  query: string,
  requests: SuggestionRequest[],
): SchemaFetchResponse[] {
  const queryLower = query.toLowerCase();
  const queryTrigrams = trigrams(query);
  const runLength = longestAlphanumericRun(query);

  return requests.flatMap((request) => {
    const matched: { response: SchemaFetchResponse; nameLower: string }[] = [];
    for (const obj of index.objects) {
      if (!matchesScope(obj, request)) continue;
      const score = scoreMatch(obj, queryLower, queryTrigrams, runLength);
      if (score === null) continue;
      matched.push({
        response: { ...obj.response, score },
        nameLower: obj.nameLower,
      });
    }

    matched.sort((a, b) => {
      const scoreDiff = (b.response.score ?? 0) - (a.response.score ?? 0);
      if (scoreDiff !== 0) return scoreDiff;
      return a.nameLower < b.nameLower ? -1 : a.nameLower > b.nameLower ? 1 : 0;
    });

    return matched.slice(0, RESULT_LIMIT).map((m) => m.response);
  });
}

// findTable resolves a table or view by exact (case-insensitive) name within an
// optional schema, returning its response (used by the autocomplete library's
// fetchTable to distinguish a table from a view). It operates on the flat
// response list rather than the trigram index, so it can run cheaply on the
// main thread without a worker round-trip. Returns undefined if not found.
export function findTable(
  responses: SchemaFetchResponse[],
  name: string | undefined,
  database: string | undefined,
  schema: string | undefined,
): SchemaFetchResponse | undefined {
  if (!name) return undefined;
  const nameLower = name.toLowerCase();
  const schemaLower = schema?.toLowerCase();
  const databaseLower = database?.toLowerCase();
  return responses.find((response) => {
    if (response.type !== 'table' && response.type !== 'view') return false;
    if (response.name.toLowerCase() !== nameLower) return false;
    if (schemaLower && response.schema?.toLowerCase() !== schemaLower) {
      return false;
    }
    if (databaseLower && response.database?.toLowerCase() !== databaseLower) {
      return false;
    }
    return true;
  });
}
