import { beforeEach, describe, expect, mock, test } from 'bun:test';

import {
  MAX_ADDITIONAL_RUNS,
  MAX_CHART_CONFIG_HISTORY_ENTRIES,
  MAX_QUERY_HISTORY_ENTRIES,
  type PersistedState,
  useServeStore,
} from './store';

// The store persists via a debounced fetch to /api/state; stub it so tests
// don't make real network calls.
globalThis.fetch = mock(async () => new Response(null, { status: 204 }));

// hydrate() reads window.location/history for the selected db id; stub a
// minimal window so the store can run outside a DOM.
// biome-ignore lint/suspicious/noExplicitAny: minimal test stub.
(globalThis as any).window = {
  location: { search: '', pathname: '/' },
  history: { replaceState: () => {} },
};

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

describe('hydrate', () => {
  const hydrate = (saved: PersistedState) =>
    useServeStore.getState().hydrate(saved);

  test('keeps a known resultView', () => {
    hydrate({ resultView: 'chart_editor' });
    expect(useServeStore.getState().resultView).toBe('chart_editor');
  });

  test('falls back to table for an unknown resultView', () => {
    // e.g. state written by an older/incompatible build (the editor view was
    // once named 'editor').
    hydrate({ resultView: 'editor' as never });
    expect(useServeStore.getState().resultView).toBe('table');
  });

  test('falls back to table when resultView is missing', () => {
    hydrate({});
    expect(useServeStore.getState().resultView).toBe('table');
  });
});

describe('chart config history', () => {
  beforeEach(() => {
    useServeStore.setState({ chartConfigHistory: [] });
  });

  const history = () => useServeStore.getState().chartConfigHistory;
  const add = (config: string) =>
    useServeStore.getState().addChartConfigHistoryEntry(config);

  test('adds entries newest first', () => {
    add('a');
    add('b');
    expect(history().map((e) => e.config)).toEqual(['b', 'a']);
  });

  test('ignores blank/whitespace-only config', () => {
    add('   \n  ');
    expect(history()).toHaveLength(0);
  });

  test('is a no-op when the config already tops the history', () => {
    add('a');
    const tsBefore = history()[0].ts;
    add('  a  '); // same (whitespace-insensitive) as the top
    expect(history()).toHaveLength(1);
    expect(history()[0].ts).toBe(tsBefore);
  });

  test('promotes (dedups) a re-added non-top config to the top', () => {
    add('a');
    add('b');
    add('c');
    add('a'); // 'a' already exists lower down
    expect(history().map((e) => e.config)).toEqual(['a', 'c', 'b']);
    expect(history()).toHaveLength(3);
  });

  test('caps the number of entries, dropping the oldest', () => {
    for (let i = 0; i < MAX_CHART_CONFIG_HISTORY_ENTRIES + 10; i++) {
      add(`config ${i}`);
    }
    const entries = history();
    expect(entries).toHaveLength(MAX_CHART_CONFIG_HISTORY_ENTRIES);
    expect(entries[0].config).toBe(
      `config ${MAX_CHART_CONFIG_HISTORY_ENTRIES + 9}`,
    );
    expect(entries[entries.length - 1].config).toBe('config 10');
  });

  test('removeChartConfigHistoryEntry removes by index', () => {
    add('a');
    add('b');
    useServeStore.getState().removeChartConfigHistoryEntry(0);
    expect(history().map((e) => e.config)).toEqual(['a']);
  });

  test('clearChartConfigHistory empties the list', () => {
    add('a');
    useServeStore.getState().clearChartConfigHistory();
    expect(history()).toHaveLength(0);
  });
});
