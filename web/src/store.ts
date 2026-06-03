import { create } from 'zustand';

export interface PersistedState {
  selectedDatabaseId?: string;
  editorHeight?: number;
  editorSql?: string;
  schemaPaneWidth?: number;
  schemaPaneVisible?: boolean;
  schemaTreeExpanded?: Record<string, string[]>;
}

interface ServeStore {
  hydrated: boolean;
  selectedDatabaseId: string | null;
  editorHeight: number;
  editorSql: string;
  schemaPaneWidth: number;
  schemaPaneVisible: boolean;
  schemaTreeExpanded: Record<string, string[]>;
  hydrate: (saved: PersistedState) => void;
  setSelectedDatabaseId: (id: string | null) => void;
  setEditorSql: (sql: string) => void;
  appendEditorSql: (sql: string) => void;
  setEditorHeight: (height: number) => void;
  setSchemaPaneWidth: (width: number) => void;
  setSchemaPaneVisible: (visible: boolean) => void;
  toggleSchemaNode: (databaseId: string, key: string) => void;
  setSchemaExpanded: (databaseId: string, keys: string[]) => void;
}

export const DEFAULT_EDITOR_HEIGHT = 240;
export const DEFAULT_SCHEMA_PANE_WIDTH = 280;
export const MIN_SCHEMA_PANE_WIDTH = 200;
export const MAX_SCHEMA_PANE_WIDTH = 600;

function getUrlDbId(): string | null {
  return new URLSearchParams(window.location.search).get('db');
}

function setUrlDbId(id: string | null) {
  const url = new URL(window.location.href);
  if (id) url.searchParams.set('db', id);
  else url.searchParams.delete('db');
  window.history.replaceState(null, '', url.toString());
}

let saveTimer: ReturnType<typeof setTimeout> | null = null;
const SAVE_DEBOUNCE_MS = 400;

function schedulePersist(snapshot: PersistedState) {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    saveTimer = null;
    void fetch('/api/state', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(snapshot),
    });
  }, SAVE_DEBOUNCE_MS);
}

function snapshotFor(store: ServeStore): PersistedState {
  return {
    selectedDatabaseId: store.selectedDatabaseId ?? undefined,
    editorSql: store.editorSql,
    editorHeight: store.editorHeight,
    schemaPaneWidth: store.schemaPaneWidth,
    schemaPaneVisible: store.schemaPaneVisible,
    schemaTreeExpanded: store.schemaTreeExpanded,
  };
}

function clamp(value: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, value));
}

export const useServeStore = create<ServeStore>((set, get) => ({
  hydrated: false,
  selectedDatabaseId: null,
  editorHeight: DEFAULT_EDITOR_HEIGHT,
  editorSql: '',
  schemaPaneWidth: DEFAULT_SCHEMA_PANE_WIDTH,
  schemaPaneVisible: true,
  schemaTreeExpanded: {},
  hydrate: (saved) => {
    const selectedDatabaseId = getUrlDbId() ?? saved.selectedDatabaseId ?? null;
    if (selectedDatabaseId) setUrlDbId(selectedDatabaseId);
    set({
      hydrated: true,
      selectedDatabaseId,
      editorSql: saved.editorSql ?? '',
      editorHeight: saved.editorHeight ?? DEFAULT_EDITOR_HEIGHT,
      schemaPaneWidth: clamp(
        saved.schemaPaneWidth ?? DEFAULT_SCHEMA_PANE_WIDTH,
        MIN_SCHEMA_PANE_WIDTH,
        MAX_SCHEMA_PANE_WIDTH,
      ),
      schemaPaneVisible: saved.schemaPaneVisible ?? true,
      schemaTreeExpanded: saved.schemaTreeExpanded ?? {},
    });
  },
  setSelectedDatabaseId: (id) => {
    set({ selectedDatabaseId: id });
    setUrlDbId(id);
    schedulePersist(snapshotFor(get()));
  },
  setEditorSql: (sql) => {
    set({ editorSql: sql });
    schedulePersist(snapshotFor(get()));
  },
  appendEditorSql: (sql) => {
    const current = get().editorSql;
    const next = current.trim() ? `${current.trimEnd()}\n\n${sql}` : sql;
    set({ editorSql: next });
    schedulePersist(snapshotFor(get()));
  },
  setEditorHeight: (height) => {
    set({ editorHeight: height });
    schedulePersist(snapshotFor(get()));
  },
  setSchemaPaneWidth: (width) => {
    set({
      schemaPaneWidth: clamp(
        Math.round(width),
        MIN_SCHEMA_PANE_WIDTH,
        MAX_SCHEMA_PANE_WIDTH,
      ),
    });
    schedulePersist(snapshotFor(get()));
  },
  setSchemaPaneVisible: (visible) => {
    set({ schemaPaneVisible: visible });
    schedulePersist(snapshotFor(get()));
  },
  toggleSchemaNode: (databaseId, key) => {
    const prev = get().schemaTreeExpanded[databaseId] ?? [];
    const next = prev.includes(key)
      ? prev.filter((k) => k !== key)
      : [...prev, key];
    set({
      schemaTreeExpanded: { ...get().schemaTreeExpanded, [databaseId]: next },
    });
    schedulePersist(snapshotFor(get()));
  },
  setSchemaExpanded: (databaseId, keys) => {
    set({
      schemaTreeExpanded: { ...get().schemaTreeExpanded, [databaseId]: keys },
    });
    schedulePersist(snapshotFor(get()));
  },
}));
