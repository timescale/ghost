import { describe, expect, test } from 'bun:test';
import type { SchemaFetchResponse } from '@timescale/popsql-query-widget-cdn';

import { buildIndex, findTable, searchIndex } from './search';

const responses: SchemaFetchResponse[] = [
  { type: 'table', name: 'users', database: 'tsdb', schema: 'public' },
  { type: 'table', name: 'user_sessions', database: 'tsdb', schema: 'public' },
  { type: 'table', name: 'orders', database: 'tsdb', schema: 'public' },
  { type: 'view', name: 'active_users', database: 'tsdb', schema: 'public' },
  {
    type: 'column',
    name: 'user_id',
    database: 'tsdb',
    schema: 'public',
    table: 'orders',
  },
  {
    type: 'column',
    name: 'email',
    database: 'tsdb',
    schema: 'analytics',
    table: 'users',
  },
];

const index = buildIndex(responses);
const names = (rows: SchemaFetchResponse[]) => rows.map((r) => r.name);

describe('searchIndex tiers', () => {
  test('empty query lists everything in scope', () => {
    const rows = searchIndex(index, '', [{ type: ['table'] }]);
    expect(names(rows).sort()).toEqual(['orders', 'user_sessions', 'users']);
  });

  test('short query (<=2 chars) matches by prefix', () => {
    const rows = searchIndex(index, 'us', [{ type: ['table'] }]);
    // 'users' and 'user_sessions' start with 'us'; 'orders' contains 'rs' but
    // does not start with 'us'.
    expect(names(rows).sort()).toEqual(['user_sessions', 'users']);
  });

  test('3-4 char query matches anywhere as a substring', () => {
    const rows = searchIndex(index, 'ser', [{ type: ['table', 'view'] }]);
    // substring 'ser' appears in 'users', 'user_sessions', 'active_users'.
    expect(names(rows).sort()).toEqual([
      'active_users',
      'user_sessions',
      'users',
    ]);
  });

  test('>=5 char query matches by substring or fuzzy similarity', () => {
    // 'userss' is not a substring of anything, but is fuzzily close to
    // 'users' / 'user_sessions' (word_similarity >= 0.5).
    const rows = searchIndex(index, 'userss', [{ type: ['table'] }]);
    expect(names(rows)).toContain('users');
    expect(names(rows)).not.toContain('orders');
  });

  test('ranks by score descending then name ascending', () => {
    const rows = searchIndex(index, 'user', [{ type: ['table', 'view'] }]);
    // word_similarity is highest when the query is a complete word in the
    // name: 'user_sessions' contains the whole word 'user' (score 1.0), so it
    // outranks 'users' and 'active_users' (0.8 each — the 'er ' word-boundary
    // trigram differs), which then tie-break by name ascending. This mirrors
    // pg_trgm word_similarity, the behavior popsql relies on.
    expect(names(rows)).toEqual(['user_sessions', 'active_users', 'users']);
  });
});

describe('searchIndex scope filters', () => {
  test('filters by type', () => {
    const rows = searchIndex(index, '', [{ type: ['view'] }]);
    expect(names(rows)).toEqual(['active_users']);
  });

  test('filters by schema (case-insensitive)', () => {
    const rows = searchIndex(index, '', [
      { type: ['column'], schema: 'ANALYTICS' },
    ]);
    expect(names(rows)).toEqual(['email']);
  });

  test('filters by table', () => {
    const rows = searchIndex(index, '', [
      { type: ['column'], table: 'orders' },
    ]);
    expect(names(rows)).toEqual(['user_id']);
  });

  test('concatenates results across multiple requests', () => {
    const rows = searchIndex(index, '', [
      { type: ['view'] },
      { type: ['column'], table: 'orders' },
    ]);
    expect(names(rows)).toEqual(['active_users', 'user_id']);
  });
});

describe('findTable', () => {
  test('resolves a table by exact name', () => {
    expect(findTable(responses, 'users', 'tsdb', 'public')?.type).toBe('table');
  });

  test('resolves a view by exact name', () => {
    expect(
      findTable(responses, 'active_users', undefined, undefined)?.type,
    ).toBe('view');
  });

  test('is case-insensitive and respects the schema filter', () => {
    expect(findTable(responses, 'USERS', undefined, 'public')?.name).toBe(
      'users',
    );
    expect(findTable(responses, 'users', undefined, 'nope')).toBeUndefined();
  });

  test('returns undefined for unknown or non-relation names', () => {
    expect(
      findTable(responses, 'missing', undefined, undefined),
    ).toBeUndefined();
    expect(
      findTable(responses, 'user_id', undefined, undefined),
    ).toBeUndefined();
  });
});
