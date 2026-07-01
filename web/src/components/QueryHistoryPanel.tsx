import { ResultsCacheContext } from '@timescale/popsql-query-widget-cdn';
import { useContext, useMemo, useState } from 'react';

import { deleteRun, type ResultsCacheClient } from '../agent/runData';
import { type QueryHistoryEntry, useServeStore } from '../store';
import { formatAbsoluteTime, formatRelativeTime } from '../util/time';
import type { ResultView } from './chart/types';
import { Icon } from './Icon';
import { QueryHistoryDetail } from './QueryHistoryDetail';

interface Props {
  // Make the selected run the active run in the main view (editor + results +
  // chart), then close the modal.
  onOpen: (entry: QueryHistoryEntry, view: ResultView, config: string) => void;
}

// A one-line preview of a run's SQL for the list (whitespace collapsed).
function previewSql(sql: string): string {
  return sql.trim().replace(/\s+/g, ' ');
}

// QueryHistoryPanel lists distinct query runs (newest first), each kept in the
// in-memory results cache. Selecting one shows it in the query widget's own
// read-only editor + results grid (see QueryHistoryDetail), with a button to make
// it the active run in the main view. Each entry can be deleted, which evicts
// its results from the cache. Unlike query/chart history, query history is never
// persisted and is capped at the server's ui_query_history_limit (oldest runs
// evicted).
export function QueryHistoryPanel({ onOpen }: Props) {
  const queryHistory = useServeStore((s) => s.queryHistory);
  const removeEntry = useServeStore((s) => s.removeQueryHistoryEntry);
  const clearHistory = useServeStore((s) => s.clearQueryHistory);
  // The widget's in-process results cache, used to evict a deleted run's rows.
  const { client } = useContext(ResultsCacheContext) as {
    client: ResultsCacheClient | null;
  };

  const [selectedIndex, setSelectedIndex] = useState(0);
  // runId of the entry whose delete is awaiting inline confirmation, if any.
  const [confirmingRemove, setConfirmingRemove] = useState<string | null>(null);
  const [confirmingClear, setConfirmingClear] = useState(false);

  // Best-effort eviction of a run's cached results from the widget cache.
  const evict = (runId: string) => {
    if (!client) return;
    void deleteRun(client, runId).catch(() => {
      // A failed delete only leaks a cached run, reclaimed on reload.
    });
  };

  const handleClear = () => {
    // Evict every run's cached results, then empty the history.
    for (const runId of clearHistory()) evict(runId);
    setConfirmingClear(false);
  };

  // Clamp the selection so eviction (which trims the oldest) can't leave it
  // pointing past the end of the list.
  const activeIndex = Math.min(
    selectedIndex,
    Math.max(0, queryHistory.length - 1),
  );
  const selected = queryHistory[activeIndex];

  // Recompute "now" once per render so all relative times share a reference.
  const now = useMemo(() => Date.now(), []);

  const handleRemove = (index: number, runId: string) => {
    // Evict the run's cached results (best effort), then drop the entry.
    evict(runId);
    removeEntry(runId);
    if (index < selectedIndex) setSelectedIndex((i) => i - 1);
    setConfirmingRemove(null);
  };

  if (queryHistory.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center p-8 text-sm text-slate-500">
        No query history yet. Each query you run is kept here (with its results)
        until it ages out.
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: list of runs (newest first). */}
      <div className="flex w-80 min-w-72 flex-col border-r border-slate-200">
        <ul className="min-h-0 flex-1 overflow-auto">
          {queryHistory.map((entry, index) => {
            const active = index === activeIndex;
            return (
              <li key={entry.runId}>
                {/* The whole row is the clickable target; the nested delete/
                    confirm buttons stop propagation so they don't also select
                    the row. */}
                {/* biome-ignore lint/a11y/useSemanticElements: a native <button> can't be used because the row contains nested action buttons (remove/confirm), which is invalid HTML; the role/tabIndex/keydown handler provide equivalent button semantics */}
                <div
                  role="button"
                  tabIndex={0}
                  onClick={() => setSelectedIndex(index)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      setSelectedIndex(index);
                    }
                  }}
                  className={`group flex w-full cursor-pointer items-center gap-2 border-b border-slate-100 px-3 py-2 text-left ${
                    active ? 'bg-slate-100' : 'hover:bg-slate-50'
                  }`}
                >
                  <span className="flex min-w-0 flex-1 flex-col items-start">
                    <span className="flex w-full items-center gap-1.5">
                      <Icon
                        name={entry.success ? 'check' : 'x'}
                        size="xs"
                        color={entry.success ? 'green' : 'red'}
                      />
                      <span
                        className="truncate font-mono text-xs text-slate-700"
                        title={previewSql(entry.sql)}
                      >
                        {previewSql(entry.sql)}
                      </span>
                    </span>
                    <span className="mt-0.5 flex items-center gap-1.5 text-[11px] text-slate-400">
                      <span title={formatAbsoluteTime(entry.ts)}>
                        {formatRelativeTime(entry.ts, now)}
                      </span>
                      <span>· {entry.databaseName}</span>
                      {entry.success ? (
                        <span>
                          · {entry.rowCount} row
                          {entry.rowCount === 1 ? '' : 's'}
                        </span>
                      ) : null}
                    </span>
                  </span>
                  {confirmingRemove === entry.runId ? (
                    <span className="flex items-center gap-1">
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleRemove(index, entry.runId);
                        }}
                        aria-label="Confirm delete"
                        title="Confirm delete"
                        className="rounded border border-red-300 bg-red-50 p-1 text-red-600 hover:bg-red-100"
                      >
                        <Icon name="check" size="sm" color="current" />
                      </button>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          setConfirmingRemove(null);
                        }}
                        aria-label="Cancel delete"
                        title="Cancel delete"
                        className="rounded border border-slate-300 bg-white p-1 text-slate-600 hover:bg-slate-50"
                      >
                        <Icon name="x" size="sm" color="current" />
                      </button>
                    </span>
                  ) : (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        setConfirmingRemove(entry.runId);
                      }}
                      aria-label="Delete run (evict from cache)"
                      title="Delete run (evict from cache)"
                      className="rounded p-1 text-slate-400 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-slate-200 hover:text-red-600"
                    >
                      <Icon name="trash" size="sm" color="current" />
                    </button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
        <div className="border-t border-slate-200 p-2">
          {confirmingClear ? (
            <div className="flex items-center justify-between gap-2 text-xs text-slate-600">
              <span>Clear all query history?</span>
              <span className="flex gap-1">
                <button
                  type="button"
                  onClick={handleClear}
                  className="rounded border border-red-300 bg-red-50 px-2 py-1 text-red-600 hover:bg-red-100"
                >
                  Clear
                </button>
                <button
                  type="button"
                  onClick={() => setConfirmingClear(false)}
                  className="rounded border border-slate-300 bg-white px-2 py-1 text-slate-600 hover:bg-slate-50"
                >
                  Cancel
                </button>
              </span>
            </div>
          ) : (
            <button
              type="button"
              onClick={() => setConfirmingClear(true)}
              className="flex w-full items-center justify-center gap-1.5 rounded border border-slate-300 bg-white px-2 py-1.5 text-xs text-slate-600 hover:bg-slate-50 hover:text-slate-800"
            >
              <Icon name="trash" size="sm" color="current" />
              Clear all query history
            </button>
          )}
        </div>
      </div>

      {/* Right: the selected run in the widget's own editor + results grid.
          Not remounted per selection (its runId prop drives the grid); it
          re-seeds its preview state from the entry instead. */}
      {selected ? (
        <QueryHistoryDetail entry={selected} onOpen={onOpen} />
      ) : null}
    </div>
  );
}
