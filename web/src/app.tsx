import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
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

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path}: ${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

const READY_STATUSES = new Set(['ready', 'running']);

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
  const [selectedId, setSelectedId] = useState<string | null>(getUrlDbId);
  const databases = useQuery({
    queryKey: ['databases'],
    queryFn: async () => {
      const data = await fetchJSON<Database[]>('/api/databases');
      if (!getUrlDbId()) {
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
        {bootstrap.isError ? (
          <div className="text-red-600">Failed to load bootstrap config</div>
        ) : !selected ? (
          <div className="text-slate-500">Select a database to run queries.</div>
        ) : !bootstrap.data ? (
          <div className="text-slate-500">Loading…</div>
        ) : (
          <QueryPanel
            projectId={bootstrap.data.projectId}
            databaseId={selected.id}
            databaseName={selected.name}
          />
        )}
      </main>
    </div>
  );
}
