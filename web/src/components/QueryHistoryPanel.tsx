import { ResultsCacheContext } from '@timescale/popsql-query-widget-cdn';
import { useContext, useMemo } from 'react';

import { deleteRun, type ResultsCacheClient } from '../agent/runData';
import { type QueryHistoryEntry, useServeStore } from '../store';
import { formatAbsoluteTime, formatRelativeTime } from '../util/time';
import type { ResultView } from './chart/types';
import { ClearHistoryFooter, HistoryListRow } from './history/HistoryList';
import { useHistorySelection } from './history/useHistorySelection';
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

  const { activeIndex, setSelectedIndex, adjustForRemoval } =
    useHistorySelection(queryHistory.length);
  const selected = queryHistory[activeIndex];

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
  };

  // Recompute "now" once per render so all relative times share a reference.
  const now = useMemo(() => Date.now(), []);

  const handleRemove = (index: number, runId: string) => {
    // Evict the run's cached results (best effort), then drop the entry.
    evict(runId);
    removeEntry(runId);
    adjustForRemoval(index);
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
          {queryHistory.map((entry, index) => (
            <HistoryListRow
              key={entry.runId}
              active={index === activeIndex}
              onSelect={() => setSelectedIndex(index)}
              onRemove={() => handleRemove(index, entry.runId)}
              removeLabel="Delete run (evict from cache)"
            >
              <span className="flex w-full items-center gap-1.5">
                <Icon
                  name={entry.status === 'success' ? 'check' : 'x'}
                  size="xs"
                  color={
                    entry.status === 'success'
                      ? 'green'
                      : entry.status === 'canceled'
                        ? 'gray'
                        : 'red'
                  }
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
                {entry.status === 'canceled' ? (
                  <span>· canceled</span>
                ) : entry.status === 'success' ? (
                  <span>
                    · {entry.rowCount} row
                    {entry.rowCount === 1 ? '' : 's'}
                  </span>
                ) : null}
              </span>
            </HistoryListRow>
          ))}
        </ul>
        <ClearHistoryFooter
          label="Clear all query history"
          onClear={handleClear}
        />
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
