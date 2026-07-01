import { useMemo } from 'react';

import { evictRuns } from '../agent/runData';
import { useResultsCacheClient } from '../agent/useResultsCacheClient';
import { type QueryHistoryEntry, useServeStore } from '../store';
import { formatAbsoluteTime, formatRelativeTime } from '../util/time';
import type { ResultView } from './chart/types';
import { ClearHistoryFooter, HistoryListRow } from './history/HistoryList';
import { previewText } from './history/previewText';
import { useHistorySelection } from './history/useHistorySelection';
import { Icon } from './Icon';
import { QueryHistoryDetail } from './QueryHistoryDetail';

interface Props {
  // Make the selected run the active run in the main view (editor + results +
  // chart), then close the modal.
  onOpen: (entry: QueryHistoryEntry, view: ResultView, config: string) => void;
  // Notify the parent that these runIds' cached results have been evicted (via
  // delete or clear), so it can drop any main-view references to them (the
  // results grid / chart) before they resolve to a missing cache entry.
  onRunsEvicted: (runIds: string[]) => void;
}

// QueryHistoryPanel lists distinct query runs (newest first), each kept in the
// in-memory results cache. Selecting one shows it in the query widget's own
// read-only editor + results grid (see QueryHistoryDetail), with a button to make
// it the active run in the main view. Each entry can be deleted, which evicts
// its results from the cache. Unlike query/chart history, query history is never
// persisted and is capped at the server's ui_query_history_limit (oldest runs
// evicted).
export function QueryHistoryPanel({ onOpen, onRunsEvicted }: Props) {
  const queryHistory = useServeStore((s) => s.queryHistory);
  const removeEntry = useServeStore((s) => s.removeQueryHistoryEntry);
  const clearHistory = useServeStore((s) => s.clearQueryHistory);
  // The widget's in-process results cache, used to evict a deleted run's rows.
  const client = useResultsCacheClient();

  const { activeId, setSelectedId } = useHistorySelection(
    useMemo(() => queryHistory.map((e) => e.runId), [queryHistory]),
  );
  const selected = queryHistory.find((e) => e.runId === activeId) ?? null;

  const handleClear = () => {
    // Evict every run's cached results, then empty the history.
    const runIds = clearHistory();
    evictRuns(client, runIds);
    // Drop any main-view references to the now-evicted runs.
    onRunsEvicted(runIds);
  };

  // Recompute "now" once per render so all relative times share a reference.
  const now = useMemo(() => Date.now(), []);

  const handleRemove = (runId: string) => {
    // Evict the run's cached results (best effort), then drop the entry.
    evictRuns(client, [runId]);
    removeEntry(runId);
    // Drop any main-view reference to this now-evicted run.
    onRunsEvicted([runId]);
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
          {queryHistory.map((entry) => (
            <HistoryListRow
              key={entry.runId}
              active={entry.runId === activeId}
              onSelect={() => setSelectedId(entry.runId)}
              onRemove={() => handleRemove(entry.runId)}
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
                  title={previewText(entry.sql)}
                >
                  {previewText(entry.sql)}
                </span>
              </span>
              <span className="mt-0.5 flex items-center gap-1.5 text-[11px] text-slate-400">
                <span title={formatAbsoluteTime(entry.ts)}>
                  {formatRelativeTime(entry.ts, now)}
                </span>
                <span>· {entry.databaseName}</span>
                {entry.status === 'canceled' ? (
                  <span>· canceled</span>
                ) : entry.status === 'failed' ? (
                  <span>· failed</span>
                ) : (
                  <span>
                    · {entry.rowCount} row
                    {entry.rowCount === 1 ? '' : 's'}
                  </span>
                )}
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
