import { beforeEach, describe, expect, mock, test } from 'bun:test';

import {
  DEFAULT_QUERY_HISTORY_LIMIT,
  MAX_CHART_CONFIG_HISTORY_ENTRIES,
  MAX_EDITOR_HISTORY_ENTRIES,
  type PersistedState,
  type QueryHistoryEntry,
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

describe('editor history', () => {
  beforeEach(() => {
    useServeStore.setState({ editorHistory: [] });
  });

  const history = () => useServeStore.getState().editorHistory;
  const add = (sql: string) =>
    useServeStore.getState().addEditorHistoryEntry(sql);

  test('adds entries newest first', () => {
    add('SELECT 1');
    add('SELECT 2');
    expect(history().map((e) => e.sql)).toEqual(['SELECT 2', 'SELECT 1']);
  });

  test('ignores blank/whitespace-only SQL', () => {
    add('   \n  ');
    expect(history()).toHaveLength(0);
  });

  test('is a no-op when the content already tops the history', () => {
    add('SELECT 1');
    const tsBefore = history()[0].ts;
    add('  SELECT 1  '); // same (whitespace-insensitive) as the top
    expect(history()).toHaveLength(1);
    expect(history()[0].ts).toBe(tsBefore);
  });

  test('promotes (dedups) re-added non-top content to the top', () => {
    add('SELECT 1');
    add('SELECT 2');
    add('SELECT 1'); // 'SELECT 1' already exists lower down
    expect(history().map((e) => e.sql)).toEqual(['SELECT 1', 'SELECT 2']);
    expect(history()).toHaveLength(2);
  });

  test('caps the number of entries, dropping the oldest', () => {
    for (let i = 0; i < MAX_EDITOR_HISTORY_ENTRIES + 10; i++) {
      add(`SELECT ${i}`);
    }
    const entries = history();
    expect(entries).toHaveLength(MAX_EDITOR_HISTORY_ENTRIES);
    expect(entries[0].sql).toBe(`SELECT ${MAX_EDITOR_HISTORY_ENTRIES + 9}`);
    expect(entries[entries.length - 1].sql).toBe('SELECT 10');
  });

  test('removeEditorHistoryEntry removes by index', () => {
    add('SELECT 1');
    add('SELECT 2');
    useServeStore.getState().removeEditorHistoryEntry(0);
    expect(history().map((e) => e.sql)).toEqual(['SELECT 1']);
  });

  test('clearEditorHistory empties the list', () => {
    add('SELECT 1');
    useServeStore.getState().clearEditorHistory();
    expect(history()).toHaveLength(0);
  });
});

describe('query history', () => {
  beforeEach(() => {
    useServeStore.setState({
      queryHistory: [],
      queryHistoryLimit: DEFAULT_QUERY_HISTORY_LIMIT,
    });
  });

  const history = () => useServeStore.getState().queryHistory;
  const add = (entry: Partial<QueryHistoryEntry> & { runId: string }) =>
    useServeStore.getState().addQueryHistoryEntry({
      databaseId: 'db1',
      databaseName: 'db one',
      sql: 'SELECT 1',
      chartConfig: '',
      ts: Date.now(),
      success: true,
      rowCount: 1,
      ...entry,
    });

  test('adds entries newest first and evicts nothing under the limit', () => {
    expect(add({ runId: 'a' })).toEqual([]);
    expect(add({ runId: 'b' })).toEqual([]);
    expect(history().map((e) => e.runId)).toEqual(['b', 'a']);
  });

  test('does not deduplicate identical SQL across distinct runs', () => {
    add({ runId: 'a', sql: 'SELECT 1' });
    add({ runId: 'b', sql: 'SELECT 1' });
    expect(history().map((e) => e.runId)).toEqual(['b', 'a']);
  });

  test('evicts the oldest runId when exceeding the limit', () => {
    useServeStore.setState({ queryHistoryLimit: 2 });
    expect(add({ runId: 'a' })).toEqual([]);
    expect(add({ runId: 'b' })).toEqual([]);
    expect(add({ runId: 'c' })).toEqual(['a']);
    expect(history().map((e) => e.runId)).toEqual(['c', 'b']);
  });

  test('removeQueryHistoryEntry removes by runId', () => {
    add({ runId: 'a' });
    add({ runId: 'b' });
    useServeStore.getState().removeQueryHistoryEntry('b');
    expect(history().map((e) => e.runId)).toEqual(['a']);
  });

  test('clearQueryHistory empties the list and returns the runIds', () => {
    add({ runId: 'a' });
    add({ runId: 'b' });
    expect(useServeStore.getState().clearQueryHistory()).toEqual(['b', 'a']);
    expect(history()).toHaveLength(0);
  });

  test('setQueryHistoryLimit trims and returns the evicted runIds', () => {
    add({ runId: 'a' });
    add({ runId: 'b' });
    add({ runId: 'c' });
    // History is [c, b, a]; trimming to 1 evicts b and a.
    expect(useServeStore.getState().setQueryHistoryLimit(1)).toEqual([
      'b',
      'a',
    ]);
    expect(history().map((e) => e.runId)).toEqual(['c']);
    expect(useServeStore.getState().queryHistoryLimit).toBe(1);
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
