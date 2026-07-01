import { create } from 'zustand';
import { DEFAULT_CHART_CONFIG } from './components/chart/defaultConfig';
import type { ResultView } from './components/chart/types';
import { debounce } from './util/debounce';
import { newId } from './util/id';

// Exhaustive map of valid result views. Typed as Record<ResultView, ...> so
// adding a new ResultView fails type checking here until it's listed.
const RESULT_VIEWS: Record<ResultView, ResultView> = {
  table: 'table',
  chart: 'chart',
  chart_editor: 'chart_editor',
};

// One entry in the editor history. Editor history is a record of the full editor
// contents over time (drafts), recorded as the user edits — not tied to runs
// (each distinct run is captured by the query history instead). Identical
// contents are deduplicated globally (re-visiting one moves it to the top
// rather than adding a duplicate), mirroring the chart config history.
export interface EditorHistoryEntry {
  // Stable, unique identity assigned when the entry is created. Used as the
  // list key and selection identity so those track the entry (not its
  // position) as the live list mutates (prepends from the recorder/agent,
  // removals). Not derived from `ts`, which can collide and changes when an
  // entry is promoted to the top on dedup.
  id: string;
  // The full editor contents at the time this snapshot was recorded.
  sql: string;
  // Epoch milliseconds when this content was last recorded.
  ts: number;
}

// Maximum number of distinct history entries to retain (oldest dropped first).
export const MAX_EDITOR_HISTORY_ENTRIES = 100;

// One entry in the chart config history. Unlike query runs, there's no discrete
// "completion" event for a config, so entries are recorded whenever an edited
// config renders successfully (debounced). Identical configs are deduplicated
// globally (re-rendering or re-applying one moves it to the top rather than
// adding a duplicate).
export interface ChartConfigHistoryEntry {
  // Stable, unique identity assigned when the entry is created (see
  // EditorHistoryEntry.id).
  id: string;
  // The full chart config source.
  config: string;
  // Epoch milliseconds when this config was last recorded (rendered/applied).
  ts: number;
}

// Maximum number of chart config history entries to retain (oldest dropped).
export const MAX_CHART_CONFIG_HISTORY_ENTRIES = 100;

// The terminal outcome of a recorded run. Canceled is a distinct third state:
// unlike a failure, a canceled run can still have produced (partial) results.
export type QueryRunStatus = 'success' | 'failed' | 'canceled';

// One entry in the query history. Unlike the query/chart config histories, run
// history is never persisted and records each distinct *run* (a single
// execution and its results). The actual result rows live in the widget's
// in-memory results cache (keyed by runId); this entry holds the metadata
// needed to list runs and re-display one. Capped at `queryHistoryLimit`
// entries: when a new run pushes past the limit, the oldest run's id is returned by
// `addQueryHistoryEntry` so the caller can evict it from the results cache.
export interface QueryHistoryEntry {
  // The widget results-cache key for this run's results.
  runId: string;
  // The database the run executed against (id + display name).
  databaseId: string;
  databaseName: string;
  // The exact SQL that was executed (a selection, if one was active, otherwise
  // the full editor contents at run time).
  sql: string;
  // The chart config in effect when the run completed. For agent-driven
  // ghost_visualize this is the config the agent applied (applied before the
  // run is recorded); for user runs it's the current editor config. Lets the
  // query-history detail re-render the run's chart exactly as it was.
  chartConfig: string;
  // Epoch milliseconds when the run completed.
  ts: number;
  // Terminal outcome of the run. A canceled run can still have (partial)
  // results cached, so it's kept in history and its cache entry is retained
  // (not deleted while it remains in history) so the widget can still display
  // whatever it produced. It's evicted only when the entry leaves history.
  status: QueryRunStatus;
  // Total number of rows the run produced (0 for a failed run; may be a partial
  // count for a canceled run).
  rowCount: number;
}

// Default number of runs to retain in memory, used until the real limit is
// loaded from /api/bootstrap (the ui_query_history_limit config option).
export const DEFAULT_QUERY_HISTORY_LIMIT = 25;

export interface PersistedState {
  selectedDatabaseId?: string;
  editorHeight?: number;
  editorSql?: string;
  schemaPaneWidth?: number;
  schemaPaneVisible?: boolean;
  schemaTreeExpanded?: Record<string, string[]>;
  showInternalObjects?: boolean;
  resultView?: ResultView;
  chartConfig?: string;
  chartEditorWidth?: number;
  // Persisted history entries may predate the `id` field (written by an older
  // build), so `id` is optional here and backfilled on hydrate.
  editorHistory?: (Omit<EditorHistoryEntry, 'id'> & { id?: string })[];
  chartConfigHistory?: (Omit<ChartConfigHistoryEntry, 'id'> & {
    id?: string;
  })[];
}

interface ServeStore {
  hydrated: boolean;
  selectedDatabaseId: string | null;
  editorHeight: number;
  editorSql: string;
  schemaPaneWidth: number;
  schemaPaneVisible: boolean;
  schemaTreeExpanded: Record<string, string[]>;
  showInternalObjects: boolean;
  resultView: ResultView;
  chartConfig: string;
  chartEditorWidth: number;
  editorHistory: EditorHistoryEntry[];
  chartConfigHistory: ChartConfigHistoryEntry[];
  // Query history is in-memory only (never persisted). Newest first.
  queryHistory: QueryHistoryEntry[];
  queryHistoryLimit: number;
  hydrate: (saved: PersistedState) => void;
  setSelectedDatabaseId: (id: string | null) => void;
  setEditorSql: (sql: string) => void;
  // Appends the given SQL to the editor contents (separated by a blank line if
  // non-empty), returning the resulting combined contents so the caller can
  // mark them as applied in editor history without recomputing the join.
  appendEditorSql: (sql: string) => string;
  setEditorHeight: (height: number) => void;
  setSchemaPaneWidth: (width: number | ((prevWidth: number) => number)) => void;
  setSchemaPaneVisible: (visible: boolean) => void;
  setShowInternalObjects: (show: boolean) => void;
  setResultView: (view: ResultView) => void;
  setChartConfig: (config: string) => void;
  setChartEditorWidth: (
    width: number | ((prevWidth: number) => number),
  ) => void;
  toggleSchemaNode: (databaseId: string, key: string) => void;
  addEditorHistoryEntry: (sql: string) => void;
  removeEditorHistoryEntry: (id: string) => void;
  clearEditorHistory: () => void;
  addChartConfigHistoryEntry: (config: string) => void;
  removeChartConfigHistoryEntry: (id: string) => void;
  clearChartConfigHistory: () => void;
  // Prepends a run and trims to the limit, returning the runIds dropped past
  // the limit (newest-first eviction order) so the caller can evict their
  // results from the widget cache. Returns an empty array when nothing is
  // evicted.
  addQueryHistoryEntry: (entry: QueryHistoryEntry) => string[];
  // Sets the retention limit (from /api/bootstrap) and trims the history to it.
  // No runs exist when this runs at startup (the query panel doesn't mount
  // until bootstrap resolves), and addQueryHistoryEntry already caps the list,
  // so the trim is only a defensive invariant — nothing to evict.
  setQueryHistoryLimit: (limit: number) => void;
  // Removes a single run from the history by runId. The caller is responsible
  // for evicting the run's cached results (deleteRun) separately.
  removeQueryHistoryEntry: (runId: string) => void;
  // Clears the entire query history, returning the runIds that were in it so
  // the caller can evict their cached results (deleteRun).
  clearQueryHistory: () => string[];
}

export const DEFAULT_EDITOR_HEIGHT = 240;
export const DEFAULT_SCHEMA_PANE_WIDTH = 280;
export const DEFAULT_CHART_EDITOR_WIDTH = 640;

function getUrlDbId(): string | null {
  return new URLSearchParams(window.location.search).get('db');
}

function setUrlDbId(id: string | null) {
  const url = new URL(window.location.href);
  if (id) url.searchParams.set('db', id);
  else url.searchParams.delete('db');
  window.history.replaceState(null, '', url.toString());
}

const persist = debounce((snapshot: PersistedState) => {
  fetch('/api/state', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(snapshot),
  }).catch(console.error);
}, 400);

function snapshotFor(store: ServeStore): PersistedState {
  return {
    selectedDatabaseId: store.selectedDatabaseId ?? undefined,
    editorSql: store.editorSql,
    editorHeight: store.editorHeight,
    schemaPaneWidth: store.schemaPaneWidth,
    schemaPaneVisible: store.schemaPaneVisible,
    schemaTreeExpanded: store.schemaTreeExpanded,
    showInternalObjects: store.showInternalObjects,
    resultView: store.resultView,
    chartConfig: store.chartConfig,
    chartEditorWidth: store.chartEditorWidth,
    editorHistory: store.editorHistory,
    chartConfigHistory: store.chartConfigHistory,
  };
}

export const useServeStore = create<ServeStore>((set, get) => ({
  hydrated: false,
  selectedDatabaseId: null,
  editorHeight: DEFAULT_EDITOR_HEIGHT,
  editorSql: '',
  schemaPaneWidth: DEFAULT_SCHEMA_PANE_WIDTH,
  schemaPaneVisible: true,
  schemaTreeExpanded: {},
  showInternalObjects: false,
  resultView: 'table',
  chartConfig: DEFAULT_CHART_CONFIG,
  chartEditorWidth: DEFAULT_CHART_EDITOR_WIDTH,
  editorHistory: [],
  chartConfigHistory: [],
  queryHistory: [],
  queryHistoryLimit: DEFAULT_QUERY_HISTORY_LIMIT,
  hydrate: (saved) => {
    const selectedDatabaseId = getUrlDbId() ?? saved.selectedDatabaseId ?? null;
    if (selectedDatabaseId) setUrlDbId(selectedDatabaseId);
    set({
      hydrated: true,
      selectedDatabaseId,
      editorSql: saved.editorSql ?? '',
      editorHeight: saved.editorHeight ?? DEFAULT_EDITOR_HEIGHT,
      schemaPaneWidth: saved.schemaPaneWidth ?? DEFAULT_SCHEMA_PANE_WIDTH,
      schemaPaneVisible: saved.schemaPaneVisible ?? true,
      schemaTreeExpanded: saved.schemaTreeExpanded ?? {},
      showInternalObjects: saved.showInternalObjects ?? false,
      resultView:
        (saved.resultView && RESULT_VIEWS[saved.resultView]) ?? 'table',
      chartConfig: saved.chartConfig ?? DEFAULT_CHART_CONFIG,
      chartEditorWidth: saved.chartEditorWidth ?? DEFAULT_CHART_EDITOR_WIDTH,
      // Backfill a stable id for any persisted entry written before ids
      // existed, so the list key/selection identity is always present.
      editorHistory: (saved.editorHistory ?? []).map((e) => ({
        ...e,
        id: e.id ?? newId(),
      })),
      chartConfigHistory: (saved.chartConfigHistory ?? []).map((e) => ({
        ...e,
        id: e.id ?? newId(),
      })),
    });
  },
  setSelectedDatabaseId: (id) => {
    set({ selectedDatabaseId: id });
    setUrlDbId(id);
    persist(snapshotFor(get()));
  },
  setEditorSql: (sql) => {
    set({ editorSql: sql });
    persist(snapshotFor(get()));
  },
  appendEditorSql: (sql) => {
    const current = get().editorSql;
    const next = current.trim() ? `${current.trimEnd()}\n\n${sql}` : sql;
    set({ editorSql: next });
    persist(snapshotFor(get()));
    return next;
  },
  setEditorHeight: (height) => {
    set({ editorHeight: height });
    persist(snapshotFor(get()));
  },
  setSchemaPaneWidth: (width) => {
    set({
      schemaPaneWidth: Math.round(
        typeof width === 'function' ? width(get().schemaPaneWidth) : width,
      ),
    });
    persist(snapshotFor(get()));
  },
  setSchemaPaneVisible: (visible) => {
    set({ schemaPaneVisible: visible });
    persist(snapshotFor(get()));
  },
  setShowInternalObjects: (show) => {
    set({ showInternalObjects: show });
    persist(snapshotFor(get()));
  },
  setResultView: (view) => {
    set({ resultView: view });
    persist(snapshotFor(get()));
  },
  setChartConfig: (config) => {
    set({ chartConfig: config });
    persist(snapshotFor(get()));
  },
  setChartEditorWidth: (width) => {
    set({
      chartEditorWidth: Math.round(
        typeof width === 'function' ? width(get().chartEditorWidth) : width,
      ),
    });
    persist(snapshotFor(get()));
  },
  addEditorHistoryEntry: (sql) => {
    const trimmed = sql.trim();
    if (!trimmed) return;
    const history = get().editorHistory;
    // Already at the top: nothing to do (avoids timestamp churn while the
    // debounced recorder fires repeatedly on the same content).
    if (history[0]?.sql.trim() === trimmed) return;
    // Global dedup + move-to-top: drop any existing identical content so
    // returning to a previous draft promotes it rather than duplicating it.
    const withoutDup = history.filter((e) => e.sql.trim() !== trimmed);
    const entry: EditorHistoryEntry = { id: newId(), sql, ts: Date.now() };
    set({
      editorHistory: [entry, ...withoutDup].slice(
        0,
        MAX_EDITOR_HISTORY_ENTRIES,
      ),
    });
    persist(snapshotFor(get()));
  },
  removeEditorHistoryEntry: (id) => {
    set({ editorHistory: get().editorHistory.filter((e) => e.id !== id) });
    persist(snapshotFor(get()));
  },
  clearEditorHistory: () => {
    set({ editorHistory: [] });
    persist(snapshotFor(get()));
  },
  addChartConfigHistoryEntry: (config) => {
    const trimmed = config.trim();
    if (!trimmed) return;
    const history = get().chartConfigHistory;
    // Already at the top: nothing to do (avoids timestamp churn while the
    // debounced recorder fires repeatedly on the same config).
    if (history[0]?.config.trim() === trimmed) return;
    // Global dedup + move-to-top: drop any existing identical config so
    // re-rendering or re-applying one promotes it rather than duplicating it.
    const withoutDup = history.filter((e) => e.config.trim() !== trimmed);
    const entry: ChartConfigHistoryEntry = {
      id: newId(),
      config,
      ts: Date.now(),
    };
    set({
      chartConfigHistory: [entry, ...withoutDup].slice(
        0,
        MAX_CHART_CONFIG_HISTORY_ENTRIES,
      ),
    });
    persist(snapshotFor(get()));
  },
  removeChartConfigHistoryEntry: (id) => {
    set({
      chartConfigHistory: get().chartConfigHistory.filter((e) => e.id !== id),
    });
    persist(snapshotFor(get()));
  },
  clearChartConfigHistory: () => {
    set({ chartConfigHistory: [] });
    persist(snapshotFor(get()));
  },
  addQueryHistoryEntry: (entry) => {
    const { queryHistory, queryHistoryLimit } = get();
    const next = [entry, ...queryHistory];
    const evicted = next.slice(queryHistoryLimit).map((e) => e.runId);
    set({ queryHistory: next.slice(0, queryHistoryLimit) });
    // Query history is intentionally not persisted, so no persist() call here.
    return evicted;
  },
  setQueryHistoryLimit: (limit) => {
    set({
      queryHistoryLimit: limit,
      queryHistory: get().queryHistory.slice(0, limit),
    });
  },
  removeQueryHistoryEntry: (runId) => {
    set({ queryHistory: get().queryHistory.filter((e) => e.runId !== runId) });
    // Query history is intentionally not persisted, so no persist() call here.
  },
  clearQueryHistory: () => {
    const runIds = get().queryHistory.map((e) => e.runId);
    set({ queryHistory: [] });
    // Query history is intentionally not persisted, so no persist() call here.
    return runIds;
  },
  toggleSchemaNode: (databaseId, key) => {
    const prev = get().schemaTreeExpanded[databaseId] ?? [];
    const next = prev.includes(key)
      ? prev.filter((k) => k !== key)
      : [...prev, key];
    set({
      schemaTreeExpanded: { ...get().schemaTreeExpanded, [databaseId]: next },
    });
    persist(snapshotFor(get()));
  },
}));
