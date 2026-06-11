import { create } from 'zustand';
import { debounce } from './util/debounce';

export interface PersistedState {
  selectedDatabaseId?: string;
  editorHeight?: number;
  editorSql?: string;
  schemaPaneWidth?: number;
  schemaPaneVisible?: boolean;
  schemaTreeExpanded?: Record<string, string[]>;
  showInternalObjects?: boolean;
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
  hydrate: (saved: PersistedState) => void;
  setSelectedDatabaseId: (id: string | null) => void;
  setEditorSql: (sql: string) => void;
  appendEditorSql: (sql: string) => void;
  setEditorHeight: (height: number) => void;
  setSchemaPaneWidth: (width: number | ((prevWidth: number) => number)) => void;
  setSchemaPaneVisible: (visible: boolean) => void;
  setShowInternalObjects: (show: boolean) => void;
  toggleSchemaNode: (databaseId: string, key: string) => void;
}

export const DEFAULT_EDITOR_HEIGHT = 240;
export const DEFAULT_SCHEMA_PANE_WIDTH = 280;

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
