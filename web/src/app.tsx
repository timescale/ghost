import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
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

export function App() {
  const bootstrap = useQuery({
    queryKey: ['bootstrap'],
    queryFn: () => fetchJSON<Bootstrap>('/api/bootstrap'),
  });
  const databases = useQuery({
    queryKey: ['databases'],
    queryFn: () => fetchJSON<Database[]>('/api/databases'),
    refetchInterval: 10_000,
  });

  const [selectedId, setSelectedId] = useState<string | null>(getUrlDbId);

  useEffect(() => {
    if (!databases.data || selectedId) return;
    const ready = databases.data.find((db) => READY_STATUSES.has(db.status));
    if (databases.data.length === 1) {
      setSelectedId(databases.data[0]!.id);
    } else if (ready && databases.data.filter((db) => READY_STATUSES.has(db.status)).length === 1) {
      setSelectedId(ready.id);
    }
  }, [databases.data, selectedId]);

  useEffect(() => {
    setUrlDbId(selectedId);
  }, [selectedId]);

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
              className="rounded border border-slate-300 bg-white px-2 py-1 text-sm focus:border-slate-500 focus:outline-none"
              value={selectedId ?? ''}
              onChange={(e) => setSelectedId(e.target.value || null)}
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
      <main className="flex flex-1 flex-col overflow-hidden p-2">
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
