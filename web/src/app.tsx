import { useQuery } from '@tanstack/react-query';
import '@timescale/popsql-query-widget-cdn/index.css';

import { QueryPanel } from './components/QueryPanel';
import { type PersistedState, useServeStore } from './store';

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

interface DatabasesResponse {
  databases: Database[];
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path}: ${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

const READY_STATUSES = new Set(['ready', 'running']);

function pickDefaultDatabaseId(databases: Database[]): string | null {
  if (databases.length === 1) return databases[0]?.id ?? null;
  const ready = databases.filter((db) => READY_STATUSES.has(db.status));
  if (ready.length === 1) return ready[0]?.id ?? null;
  return null;
}

export function App() {
  const bootstrap = useQuery({
    queryKey: ['bootstrap'],
    queryFn: () => fetchJSON<Bootstrap>('/api/bootstrap'),
  });
  const persistedState = useQuery({
    queryKey: ['state'],
    queryFn: async () => {
      const data = await fetchJSON<PersistedState>('/api/state');
      useServeStore.getState().hydrate(data);
      return data;
    },
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const hydrated = useServeStore((s) => s.hydrated);

  if (bootstrap.isError || persistedState.isError) {
    return (
      <div className="flex h-full items-center justify-center text-red-600">
        Failed to load app config
      </div>
    );
  }
  if (!bootstrap.data || !hydrated) {
    return null;
  }
  return <ReadyApp bootstrap={bootstrap.data} />;
}

interface ReadyAppProps {
  bootstrap: Bootstrap;
}

function ReadyApp({ bootstrap }: ReadyAppProps) {
  const selectedId = useServeStore((s) => s.selectedDatabaseId);
  const setSelectedDatabaseId = useServeStore((s) => s.setSelectedDatabaseId);
  const editorSql = useServeStore((s) => s.editorSql);
  const setEditorSql = useServeStore((s) => s.setEditorSql);
  const editorHeight = useServeStore((s) => s.editorHeight);
  const setEditorHeight = useServeStore((s) => s.setEditorHeight);

  const databases = useQuery({
    queryKey: ['databases'],
    queryFn: async () => {
      const { databases } =
        await fetchJSON<DatabasesResponse>('/api/databases');
      if (!useServeStore.getState().selectedDatabaseId) {
        const defaultId = pickDefaultDatabaseId(databases);
        if (defaultId)
          useServeStore.getState().setSelectedDatabaseId(defaultId);
      }
      return databases;
    },
    refetchInterval: 10_000,
  });

  const selected = databases.data?.find((db) => db.id === selectedId) ?? null;

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-2">
        <div className="font-mono text-lg font-semibold tracking-tight">
          ghost
        </div>
        <div className="flex items-center gap-2 text-sm">
          {databases.isError ? (
            <span className="text-red-600">Failed to load databases</span>
          ) : (
            <select
              name="database"
              aria-label="Database"
              className="rounded border border-slate-300 bg-white px-2 py-1 text-sm focus:border-slate-500 focus:outline-none"
              value={selectedId ?? ''}
              onChange={(e) => setSelectedDatabaseId(e.target.value || null)}
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
          <div className="text-slate-500">
            Select a database to run queries.
          </div>
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
