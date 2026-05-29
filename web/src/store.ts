import { create } from 'zustand';

export interface PersistedState {
  selectedDatabaseId?: string;
  editorHeight?: number;
  editorSql?: string;
}

interface ServeStore {
  hydrated: boolean;
  selectedDatabaseId: string | null;
  editorHeight: number;
  editorSql: string;
  hydrate: (saved: PersistedState) => void;
  setSelectedDatabaseId: (id: string | null) => void;
  setEditorSql: (sql: string) => void;
  setEditorHeight: (height: number) => void;
}

export const DEFAULT_EDITOR_HEIGHT = 240;

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
  };
}

export const useServeStore = create<ServeStore>((set, get) => ({
  hydrated: false,
  selectedDatabaseId: null,
  editorHeight: DEFAULT_EDITOR_HEIGHT,
  editorSql: '',
  hydrate: (saved) => {
    // URL takes precedence over saved state for the selected DB. Write the
    // resolved id back to the URL so the address bar always reflects the
    // active selection (matches the behavior of later setSelectedDatabaseId
    // calls).
    const selectedDatabaseId = getUrlDbId() ?? saved.selectedDatabaseId ?? null;
    if (selectedDatabaseId) setUrlDbId(selectedDatabaseId);
    set({
      hydrated: true,
      selectedDatabaseId,
      editorSql: saved.editorSql ?? '',
      editorHeight: saved.editorHeight ?? DEFAULT_EDITOR_HEIGHT,
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
  setEditorHeight: (height) => {
    set({ editorHeight: height });
    schedulePersist(snapshotFor(get()));
  },
}));
