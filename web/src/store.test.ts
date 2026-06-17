import { beforeEach, describe, expect, mock, test } from 'bun:test';

import {
  MAX_ADDITIONAL_RUNS,
  MAX_QUERY_HISTORY_ENTRIES,
  useServeStore,
} from './store';

// The store persists via a debounced fetch to /api/state; stub it so tests
// don't make real network calls.
globalThis.fetch = mock(async () => new Response(null, { status: 204 }));

describe('query history', () => {
  beforeEach(() => {
    useServeStore.setState({ queryHistory: [] });
  });

  const history = () => useServeStore.getState().queryHistory;
  const add = (sql: string, success: boolean) =>
    useServeStore.getState().addQueryHistoryEntry(sql, success);

  test('adds entries newest first', () => {
    add('SELECT 1', true);
    add('SELECT 2', true);
    expect(history().map((e) => e.sql)).toEqual(['SELECT 2', 'SELECT 1']);
  });

  test('ignores blank/whitespace-only SQL', () => {
    add('   \n  ', true);
    expect(history()).toHaveLength(0);
  });

  test('deduplicates consecutive runs of the same SQL into additionalRuns', () => {
    add('SELECT 1', true);
    add('  SELECT 1  ', false);
    expect(history()).toHaveLength(1);
    const [entry] = history();
    expect(entry.success).toBe(false);
    expect(entry.additionalRuns).toEqual([
      { ts: expect.any(Number), success: true },
    ]);
  });

  test('does not deduplicate non-consecutive runs of the same SQL', () => {
    add('SELECT 1', true);
    add('SELECT 2', true);
    add('SELECT 1', true);
    expect(history().map((e) => e.sql)).toEqual([
      'SELECT 1',
      'SELECT 2',
      'SELECT 1',
    ]);
  });

  test('caps the number of entries, dropping the oldest', () => {
    for (let i = 0; i < MAX_QUERY_HISTORY_ENTRIES + 10; i++) {
      add(`SELECT ${i}`, true);
    }
    const entries = history();
    expect(entries).toHaveLength(MAX_QUERY_HISTORY_ENTRIES);
    expect(entries[0].sql).toBe(`SELECT ${MAX_QUERY_HISTORY_ENTRIES + 9}`);
    expect(entries[entries.length - 1].sql).toBe('SELECT 10');
  });

  test('caps additionalRuns per entry', () => {
    for (let i = 0; i < MAX_ADDITIONAL_RUNS + 10; i++) {
      add('SELECT 1', true);
    }
    expect(history()).toHaveLength(1);
    expect(history()[0].additionalRuns).toHaveLength(MAX_ADDITIONAL_RUNS);
  });

  test('removeQueryHistoryEntry removes by index', () => {
    add('SELECT 1', true);
    add('SELECT 2', true);
    useServeStore.getState().removeQueryHistoryEntry(0);
    expect(history().map((e) => e.sql)).toEqual(['SELECT 1']);
  });

  test('clearQueryHistory empties the list', () => {
    add('SELECT 1', true);
    useServeStore.getState().clearQueryHistory();
    expect(history()).toHaveLength(0);
  });
});
