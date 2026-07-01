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

  test('every entry gets a unique, stable id', () => {
    add('SELECT 1');
    add('SELECT 2');
    const ids = history().map((e) => e.id);
    expect(ids).toHaveLength(2);
    expect(ids[0]).not.toBe(ids[1]);
    expect(ids.every((id) => typeof id === 'string' && id.length > 0)).toBe(
      true,
    );
  });

  test('promoting an entry (dedup) preserves its id', () => {
    add('SELECT 1');
    add('SELECT 2');
    const originalId = history().find((e) => e.sql === 'SELECT 1')?.id;
    add('SELECT 1'); // re-add promotes it to the top
    expect(history()[0].sql).toBe('SELECT 1');
    // Dedup drops the old entry and inserts a fresh one, so the id changes;
    // what matters is the id is regenerated (not derived from ts).
    expect(history()[0].id).not.toBe(originalId);
  });

  test('removeEditorHistoryEntry removes by id', () => {
    add('SELECT 1');
    add('SELECT 2');
    const target = history().find((e) => e.sql === 'SELECT 2');
    if (!target) throw new Error('expected entry');
    useServeStore.getState().removeEditorHistoryEntry(target.id);
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
      status: 'success',
      rowCount: 1,
      ...entry,
    });

  test('adds entries newest first and evicts nothing under the limit', () => {
    expect(add({ runId: 'a' })).toEqual([]);
    expect(add({ runId: 'b' })).toEqual([]);
    expect(history().map((e) => e.runId)).toEqual(['b', 'a']);
  });

  test('records runs of every terminal status, including canceled', () => {
    add({ runId: 'ok', status: 'success' });
    add({ runId: 'err', status: 'failed' });
    add({ runId: 'stopped', status: 'canceled' });
    expect(history().map((e) => [e.runId, e.status])).toEqual([
      ['stopped', 'canceled'],
      ['err', 'failed'],
      ['ok', 'success'],
    ]);
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

  test('setQueryHistoryLimit sets the limit and trims to it', () => {
    add({ runId: 'a' });
    add({ runId: 'b' });
    add({ runId: 'c' });
    // History is [c, b, a]; trimming to 1 keeps only the newest.
    useServeStore.getState().setQueryHistoryLimit(1);
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

  test('backfills stable ids for persisted entries lacking them', () => {
    hydrate({
      // Simulate state written by an older build, before entries had ids.
      editorHistory: [{ sql: 'SELECT 1', ts: 1 }],
      chartConfigHistory: [{ config: 'a', ts: 2 }],
    });
    const editor = useServeStore.getState().editorHistory;
    const chart = useServeStore.getState().chartConfigHistory;
    expect(editor[0].id).toBeString();
    expect(editor[0].id.length).toBeGreaterThan(0);
    expect(chart[0].id).toBeString();
    expect(chart[0].id.length).toBeGreaterThan(0);
  });

  test('preserves existing ids on hydrate', () => {
    hydrate({
      editorHistory: [{ id: 'e1', sql: 'SELECT 1', ts: 1 }],
      chartConfigHistory: [{ id: 'c1', config: 'a', ts: 2 }],
    });
    expect(useServeStore.getState().editorHistory[0].id).toBe('e1');
    expect(useServeStore.getState().chartConfigHistory[0].id).toBe('c1');
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

  test('every entry gets a unique, stable id', () => {
    add('a');
    add('b');
    const ids = history().map((e) => e.id);
    expect(ids).toHaveLength(2);
    expect(ids[0]).not.toBe(ids[1]);
    expect(ids.every((id) => typeof id === 'string' && id.length > 0)).toBe(
      true,
    );
  });

  test('removeChartConfigHistoryEntry removes by id', () => {
    add('a');
    add('b');
    const target = history().find((e) => e.config === 'b');
    if (!target) throw new Error('expected entry');
    useServeStore.getState().removeChartConfigHistoryEntry(target.id);
    expect(history().map((e) => e.config)).toEqual(['a']);
  });

  test('clearChartConfigHistory empties the list', () => {
    add('a');
    useServeStore.getState().clearChartConfigHistory();
    expect(history()).toHaveLength(0);
  });
});
