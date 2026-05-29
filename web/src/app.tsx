import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import '@timescale/popsql-query-widget/index.css';

import { QueryPanel } from './components/QueryPanel';

interface Bootstrap {
  projectId: string;
  version: string;
}

interface Database {
  id: string;
  name: string;
  status: string;
  type?: string;
}

interface ServeState {
  selectedDatabaseId?: string;
  editorHeight?: number;
  editorSql?: string;
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path}: ${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

async function putJSON(path: string, body: unknown): Promise<void> {
  const res = await fetch(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${path}: ${res.status} ${res.statusText}`);
}

const READY_STATUSES = new Set(['ready', 'running']);
const DEFAULT_EDITOR_HEIGHT = 240;

function getUrlDbId(): string | null {
  return new URLSearchParams(window.location.search).get('db');
}

function setUrlDbId(id: string | null) {
  const url = new URL(window.location.href);
  if (id) url.searchParams.set('db', id);
  else url.searchParams.delete('db');
  window.history.replaceState(null, '', url.toString());
}

function pickDefaultDatabaseId(databases: Database[]): string | null {
  if (databases.length === 1) return databases[0]!.id;
  const ready = databases.filter((db) => READY_STATUSES.has(db.status));
  if (ready.length === 1) return ready[0]!.id;
  return null;
}

export function App() {
  const bootstrap = useQuery({
    queryKey: ['bootstrap'],
    queryFn: () => fetchJSON<Bootstrap>('/api/bootstrap'),
  });
  const persistedState = useQuery({
    queryKey: ['state'],
    queryFn: () => fetchJSON<ServeState>('/api/state'),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });

  if (bootstrap.isError || persistedState.isError) {
    return (
      <div className="flex h-full items-center justify-center text-red-600">
        Failed to load app config
      </div>
    );
  }
  if (!bootstrap.data || !persistedState.data) {
    return null;
  }
  return <ReadyApp bootstrap={bootstrap.data} initialState={persistedState.data} />;
}

interface ReadyAppProps {
  bootstrap: Bootstrap;
  initialState: ServeState;
}

function ReadyApp({ bootstrap, initialState }: ReadyAppProps) {
  const [selectedId, setSelectedId] = useState<string | null>(() => {
    const initial = getUrlDbId() ?? initialState.selectedDatabaseId ?? null;
    if (initial) setUrlDbId(initial);
    return initial;
  });
  const [editorSql, setEditorSql] = useState<string>(initialState.editorSql ?? '');
  const [editorHeight, setEditorHeight] = useState<number>(
    initialState.editorHeight ?? DEFAULT_EDITOR_HEIGHT,
  );

  const databases = useQuery({
    queryKey: ['databases'],
    queryFn: async () => {
      const data = await fetchJSON<Database[]>('/api/databases');
      if (!getUrlDbId() && !selectedId) {
        const defaultId = pickDefaultDatabaseId(data);
        if (defaultId) {
          setSelectedId(defaultId);
          setUrlDbId(defaultId);
        }
      }
      return data;
    },
    refetchInterval: 10_000,
  });

  // Debounce-persist whenever any tracked state changes. Skip the first run
  // so we don't immediately rewrite the file with values we just read.
  const isFirstSave = useRef(true);
  useEffect(() => {
    if (isFirstSave.current) {
      isFirstSave.current = false;
      return;
    }
    const handle = setTimeout(() => {
      void putJSON('/api/state', {
        selectedDatabaseId: selectedId ?? undefined,
        editorSql,
        editorHeight,
      });
    }, 400);
    return () => clearTimeout(handle);
  }, [selectedId, editorSql, editorHeight]);

  const handleSelectionChange = (id: string | null) => {
    setSelectedId(id);
    setUrlDbId(id);
  };

  const selected = databases.data?.find((db) => db.id === selectedId) ?? null;

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-2">
        <div className="font-mono text-lg font-semibold tracking-tight">ghost</div>
        <div className="flex items-center gap-2 text-sm">
          {databases.isError ? (
            <span className="text-red-600">Failed to load databases</span>
          ) : (
            <select
              name="database"
              aria-label="Database"
              className="rounded border border-slate-300 bg-white px-2 py-1 text-sm focus:border-slate-500 focus:outline-none"
              value={selectedId ?? ''}
              onChange={(e) => handleSelectionChange(e.target.value || null)}
              disabled={databases.isLoading}
            >
              <option value="">
                {databases.isLoading ? 'Loading…' : 'Select a database…'}
              </option>
              {databases.data?.map((db) => (
                <option
                  key={db.id}
                  value={db.id}
                  disabled={!READY_STATUSES.has(db.status)}
                >
                  {db.name} ({db.status})
                </option>
              ))}
            </select>
          )}
        </div>
      </header>
      <main className="flex flex-auto flex-col overflow-hidden p-4">
        {!selected ? (
          <div className="text-slate-500">Select a database to run queries.</div>
        ) : (
          <QueryPanel
            projectId={bootstrap.projectId}
            databaseId={selected.id}
            databaseName={selected.name}
            query={editorSql}
            onQueryChange={setEditorSql}
            editorHeight={editorHeight}
            onResizeEditor={setEditorHeight}
          />
        )}
      </main>
    </div>
  );
}
