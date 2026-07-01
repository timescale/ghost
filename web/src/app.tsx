import { useQuery } from '@tanstack/react-query';
import '@timescale/popsql-query-widget-cdn/index.css';

import { AgentStatusBanner } from './agent/AgentStatusBanner';
import { DisconnectedBanner } from './agent/DisconnectedBanner';
import { useAgentBridge } from './agent/useAgentBridge';
import { QueryPanel } from './components/QueryPanel';
import { SchemaPane } from './components/SchemaPane';
import { SplitPane } from './components/SplitPane';
import { type PersistedState, useServeStore } from './store';

interface Bootstrap {
  projectId: string;
  version: string;
  readOnly: boolean;
  uiQueryHistoryLimit: number;
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

// Stable empty array so useAgentBridge doesn't see a new reference each render
// before the database list loads.
const emptyDatabases: Database[] = [];

function pickDefaultDatabaseId(databases: Database[]): string | null {
  if (databases.length === 1) return databases[0]?.id ?? null;
  const ready = databases.filter((db) => READY_STATUSES.has(db.status));
  if (ready.length === 1) return ready[0]?.id ?? null;
  return null;
}

export function App() {
  const bootstrap = useQuery({
    queryKey: ['bootstrap'],
    queryFn: async () => {
      const data = await fetchJSON<Bootstrap>('/api/bootstrap');
      // Apply the in-memory query-history retention limit from the server
      // config (ui_query_history_limit) as soon as bootstrap resolves. No runs
      // can exist yet — the query panel doesn't mount until bootstrap has
      // resolved — so this only sets the limit; there's nothing to evict.
      // Only override the client default (DEFAULT_QUERY_HISTORY_LIMIT) when the
      // server sends a usable value: a hand-edited config file bypasses the
      // CLI's positive-int validation, and an older backend may omit the field
      // entirely (arriving as undefined) — in both cases keep the default
      // rather than disabling query history with a 0/negative limit.
      if (data.uiQueryHistoryLimit > 0) {
        useServeStore.getState().setQueryHistoryLimit(data.uiQueryHistoryLimit);
      }
      return data;
    },
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
  const schemaPaneWidth = useServeStore((s) => s.schemaPaneWidth);
  const setSchemaPaneWidth = useServeStore((s) => s.setSchemaPaneWidth);
  const schemaPaneVisible = useServeStore((s) => s.schemaPaneVisible);
  const setSchemaPaneVisible = useServeStore((s) => s.setSchemaPaneVisible);

  const databases = useQuery({
    queryKey: ['databases'],
    queryFn: async () => {
      const { databases } =
        await fetchJSON<DatabasesResponse>('/api/databases');
      const selectedId = useServeStore.getState().selectedDatabaseId;
      if (!selectedId) {
        const defaultId = pickDefaultDatabaseId(databases);
        if (defaultId)
          useServeStore.getState().setSelectedDatabaseId(defaultId);
      } else if (!databases.some((db) => db.id === selectedId)) {
        // The selection doesn't match any database id. It may be a name passed
        // via the ?db= URL parameter (e.g. ?db=weather-data); now that the list
        // has loaded, resolve it to the canonical id so the selector matches and
        // the URL is normalized. If it matches neither id nor name, leave it as
        // is — the backend will surface an invalid ref when a query runs.
        const byName = databases.find((db) => db.name === selectedId);
        if (byName) useServeStore.getState().setSelectedDatabaseId(byName.id);
      }
      return databases;
    },
    refetchInterval: 10_000,
  });

  // Connect to the agent bridge so MCP tools can drive this UI. The database
  // list is passed so the dispatcher can resolve a name-or-id ref to an id.
  useAgentBridge(databases.data ?? emptyDatabases);

  const selected = databases.data?.find((db) => db.id === selectedId) ?? null;
  const selectedIsReady = selected && READY_STATUSES.has(selected.status);
  // Mount the query panel as soon as a database is selected, even before the
  // database list has loaded (or if the id isn't in the list yet). This lets
  // agent-driven queries run immediately without waiting for /api/databases:
  // the panel only needs the id, and the backend validates it when the query
  // runs. Fall back to the id as the display name until the list resolves.
  const activeDatabase =
    selected ?? (selectedId ? { id: selectedId, name: selectedId } : null);

  return (
    <div className="flex h-full flex-col">
      <DisconnectedBanner />
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-2">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => setSchemaPaneVisible(!schemaPaneVisible)}
            className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-slate-900"
            aria-label={
              schemaPaneVisible ? 'Hide schema pane' : 'Show schema pane'
            }
            title={schemaPaneVisible ? 'Hide schema pane' : 'Show schema pane'}
          >
            <SidebarIcon open={schemaPaneVisible} />
          </button>
          <div className="font-mono text-lg font-semibold tracking-tight">
            ghost
          </div>
          {bootstrap.readOnly ? (
            <span
              className="inline-flex items-center gap-1 rounded-full border border-amber-300 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700"
              title="Read-only mode is enabled. Queries run through this UI cannot modify the database."
            >
              <svg
                className="h-3 w-3"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <rect x="3" y="11" width="18" height="11" rx="2" />
                <path d="M7 11V7a5 5 0 0 1 10 0v4" />
              </svg>
              Read-only
            </span>
          ) : null}
        </div>
        <div className="flex items-center gap-2 text-sm">
          <AgentStatusBanner />
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
      <main className="flex flex-auto overflow-hidden">
        <SplitPane
          showLeft={schemaPaneVisible && !!selectedIsReady}
          leftWidth={schemaPaneWidth}
          minLeftWidth={200}
          minRightWidth={500}
          onLeftWidthChange={setSchemaPaneWidth}
          left={
            selectedIsReady ? <SchemaPane databaseId={selected.id} /> : null
          }
          right={
            <div className="flex flex-auto flex-col overflow-hidden p-4">
              {!activeDatabase ? (
                <div className="text-slate-500">
                  Select a database to run queries.
                </div>
              ) : (
                <QueryPanel
                  projectId={bootstrap.projectId}
                  databaseId={activeDatabase.id}
                  databaseName={activeDatabase.name}
                  query={editorSql}
                  onQueryChange={setEditorSql}
                  editorHeight={editorHeight}
                  onResizeEditor={setEditorHeight}
                />
              )}
            </div>
          }
        />
      </main>
    </div>
  );
}

function SidebarIcon({ open }: { open: boolean }) {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <line x1="9" y1="4" x2="9" y2="20" />
      {open ? null : <line x1="9" y1="12" x2="15" y2="12" />}
    </svg>
  );
}
