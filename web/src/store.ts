import { create } from 'zustand';
import { DEFAULT_CHART_CONFIG } from './components/chart/defaultConfig';
import type { ResultView } from './components/chart/types';
import { debounce } from './util/debounce';

// Exhaustive map of valid result views. Typed as Record<ResultView, ...> so
// adding a new ResultView fails type checking here until it's listed.
const RESULT_VIEWS: Record<ResultView, ResultView> = {
  table: 'table',
  chart: 'chart',
  chart_editor: 'chart_editor',
};

// A single execution of a query, recording when it ran and whether it
// succeeded. The SQL itself lives on the parent QueryHistoryEntry.
export interface QueryRun {
  // Epoch milliseconds when the run completed.
  ts: number;
  success: boolean;
}

// One entry in the query history. Consecutive runs of the same SQL (ignoring
// leading/trailing whitespace) are collapsed into a single entry: the most
// recent run is the entry's own `ts`/`success`, and any earlier consecutive
// runs are recorded in `additionalRuns` (newest first).
export interface QueryHistoryEntry {
  // The exact SQL that was executed (a selection, if one was active, otherwise
  // the full editor contents).
  sql: string;
  // Epoch milliseconds when the most recent run completed.
  ts: number;
  success: boolean;
  // Earlier consecutive runs of the same SQL, newest first. Omitted when there
  // was only a single run.
  additionalRuns?: QueryRun[];
}

// Maximum number of distinct history entries to retain (oldest dropped first).
export const MAX_QUERY_HISTORY_ENTRIES = 100;
// Maximum number of additional (deduplicated) runs to retain per entry.
export const MAX_ADDITIONAL_RUNS = 100;

// One entry in the chart config history. Unlike query runs, there's no discrete
// "completion" event for a config, so entries are recorded whenever an edited
// config renders successfully (debounced). Identical configs are deduplicated
// globally (re-rendering or re-applying one moves it to the top rather than
// adding a duplicate).
export interface ChartConfigHistoryEntry {
  // The full chart config source.
  config: string;
  // Epoch milliseconds when this config was last recorded (rendered/applied).
  ts: number;
}

// Maximum number of chart config history entries to retain (oldest dropped).
export const MAX_CHART_CONFIG_HISTORY_ENTRIES = 100;

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
  queryHistory?: QueryHistoryEntry[];
  chartConfigHistory?: ChartConfigHistoryEntry[];
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
  queryHistory: QueryHistoryEntry[];
  chartConfigHistory: ChartConfigHistoryEntry[];
  hydrate: (saved: PersistedState) => void;
  setSelectedDatabaseId: (id: string | null) => void;
  setEditorSql: (sql: string) => void;
  appendEditorSql: (sql: string) => void;
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
  addQueryHistoryEntry: (sql: string, success: boolean) => void;
  removeQueryHistoryEntry: (index: number) => void;
  clearQueryHistory: () => void;
  addChartConfigHistoryEntry: (config: string) => void;
  removeChartConfigHistoryEntry: (index: number) => void;
  clearChartConfigHistory: () => void;
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
    queryHistory: store.queryHistory,
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
  queryHistory: [],
  chartConfigHistory: [],
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
      queryHistory: saved.queryHistory ?? [],
      chartConfigHistory: saved.chartConfigHistory ?? [],
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
  addQueryHistoryEntry: (sql, success) => {
    const trimmed = sql.trim();
    if (!trimmed) return;
    const ts = Date.now();
    const history = get().queryHistory;
    const newest = history[0];
    // Collapse consecutive runs of the same SQL (whitespace-insensitive) into
    // the newest entry, recording the prior run in additionalRuns.
    if (newest && newest.sql.trim() === trimmed) {
      const additionalRuns = [
        { ts: newest.ts, success: newest.success },
        ...(newest.additionalRuns ?? []),
      ].slice(0, MAX_ADDITIONAL_RUNS);
      const merged: QueryHistoryEntry = { sql, ts, success, additionalRuns };
      set({ queryHistory: [merged, ...history.slice(1)] });
    } else {
      const entry: QueryHistoryEntry = { sql, ts, success };
      set({
        queryHistory: [entry, ...history].slice(0, MAX_QUERY_HISTORY_ENTRIES),
      });
    }
    persist(snapshotFor(get()));
  },
  removeQueryHistoryEntry: (index) => {
    set({ queryHistory: get().queryHistory.filter((_, i) => i !== index) });
    persist(snapshotFor(get()));
  },
  clearQueryHistory: () => {
    set({ queryHistory: [] });
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
    const entry: ChartConfigHistoryEntry = { config, ts: Date.now() };
    set({
      chartConfigHistory: [entry, ...withoutDup].slice(
        0,
        MAX_CHART_CONFIG_HISTORY_ENTRIES,
      ),
    });
    persist(snapshotFor(get()));
  },
  removeChartConfigHistoryEntry: (index) => {
    set({
      chartConfigHistory: get().chartConfigHistory.filter(
        (_, i) => i !== index,
      ),
    });
    persist(snapshotFor(get()));
  },
  clearChartConfigHistory: () => {
    set({ chartConfigHistory: [] });
    persist(snapshotFor(get()));
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
