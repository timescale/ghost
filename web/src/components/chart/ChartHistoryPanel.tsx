import { useMemo } from 'react';

import { useServeStore } from '../../store';
import { formatAbsoluteTime, formatRelativeTime } from '../../util/time';
import { CopyButton } from '../CopyButton';
import { ClearHistoryFooter, HistoryListRow } from '../history/HistoryList';
import { previewText } from '../history/previewText';
import { useHistorySelection } from '../history/useHistorySelection';
import { ChartView } from './ChartView';
import { ConfigCodeView } from './ConfigCodeView';
import type { ChartData } from './types';

interface Props {
  // Replace the current chart config with the selected one, then close.
  onApply: (config: string) => void;
  // Current query results, used to render a live preview of each historical
  // config. Null when no successful query has run yet.
  data: ChartData | null;
  loading: boolean;
  dataError: string | null;
}

// ChartHistoryPanel lists previously rendered chart configs (newest first).
// Selecting one shows its source in a read-only viewer (top right) and a live
// preview rendered against the current query results (bottom right), with
// actions to copy it, apply it (replacing the current config), or remove it. A
// "Clear all" button (with confirmation) empties the entire history. Rendered
// as a tab inside the unified HistoryModal.
export function ChartHistoryPanel({
  onApply,
  data,
  loading,
  dataError,
}: Props) {
  const history = useServeStore((s) => s.chartConfigHistory);
  const removeEntry = useServeStore((s) => s.removeChartConfigHistoryEntry);
  const clearHistory = useServeStore((s) => s.clearChartConfigHistory);

  const { activeId, setSelectedId } = useHistorySelection(
    useMemo(() => history.map((e) => e.id), [history]),
  );
  const selected = history.find((e) => e.id === activeId) ?? null;

  // Capture "now" once per mount so all relative times share a single
  // reference for the lifetime of this (short-lived) modal.
  const now = useMemo(() => Date.now(), []);

  if (history.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center p-8 text-sm text-slate-500">
        No chart config history yet. Configs you edit will appear here once they
        render successfully.
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: list of entries (newest first) plus the clear-all action. */}
      <div className="flex w-80 min-w-72 flex-col border-r border-slate-200">
        <ul className="min-h-0 flex-1 overflow-auto">
          {history.map((entry) => (
            <HistoryListRow
              // Keyed by the entry's stable id so the row (and its confirm
              // state) follows the entry as the live list mutates.
              key={entry.id}
              active={entry.id === activeId}
              onSelect={() => setSelectedId(entry.id)}
              onRemove={() => removeEntry(entry.id)}
              removeLabel="Remove from history"
            >
              <span
                className="w-full truncate font-mono text-xs text-slate-700"
                title={previewText(entry.config)}
              >
                {previewText(entry.config)}
              </span>
              <span className="mt-0.5 text-[11px] text-slate-400">
                <span title={formatAbsoluteTime(entry.ts)}>
                  {formatRelativeTime(entry.ts, now)}
                </span>
              </span>
            </HistoryListRow>
          ))}
        </ul>
        <ClearHistoryFooter
          label="Clear all chart history"
          onClear={clearHistory}
        />
      </div>

      {/* Right: read-only config viewer (top) over a live preview rendered
          against the current query results (bottom). */}
      {selected ? (
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex items-center justify-between gap-2 border-b border-slate-200 px-3 py-2">
            <span className="text-xs font-medium text-slate-500">
              Preview uses the current query results
            </span>
            <span className="flex items-center gap-2">
              <CopyButton text={selected.config} />
              <button
                type="button"
                onClick={() => onApply(selected.config)}
                className="rounded border border-slate-800 bg-slate-800 px-2 py-1 text-xs text-white hover:bg-slate-700"
              >
                Apply config
              </button>
            </span>
          </div>
          <div className="flex min-h-0 flex-1 flex-col">
            <div className="min-h-0 flex-1 border-b border-slate-200">
              <ConfigCodeView config={selected.config} />
            </div>
            <div className="flex min-h-0 flex-1 flex-col bg-white">
              <ChartView
                data={data}
                loading={loading}
                dataError={dataError}
                config={selected.config}
              />
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
