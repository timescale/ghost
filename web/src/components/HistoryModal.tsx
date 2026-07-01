import { useState } from 'react';

import type { QueryHistoryEntry } from '../store';
import { ChartHistoryPanel } from './chart/ChartHistoryPanel';
import type { ChartData, ResultView } from './chart/types';
import { EditorHistoryPanel } from './EditorHistoryPanel';
import { Modal } from './Modal';
import { QueryHistoryPanel } from './QueryHistoryPanel';

export type HistoryTab = 'editor' | 'query' | 'chart';

interface Props {
  // The tab to show first (driven by which history button was clicked).
  initialTab: HistoryTab;
  onClose: () => void;
  // Editor history tab.
  onApplyEditor: (sql: string) => void;
  onAppendEditor: (sql: string) => void;
  // Query history tab. `view`/`config` carry the result view and chart config the
  // user was previewing the run with, so opening it restores that exact view.
  onOpenRun: (
    entry: QueryHistoryEntry,
    view: ResultView,
    config: string,
  ) => void;
  // Runs whose cached results were just evicted (delete/clear in the query
  // history tab), so the parent can drop any main-view references to them.
  onRunsEvicted: (runIds: string[]) => void;
  // Chart config history tab.
  onApplyConfig: (config: string) => void;
  chartData: ChartData | null;
  chartLoading: boolean;
  chartError: string | null;
}

const TABS: { id: HistoryTab; label: string }[] = [
  { id: 'query', label: 'Query history' },
  { id: 'editor', label: 'Editor history' },
  { id: 'chart', label: 'Chart history' },
];

// HistoryModal is the unified history surface for the serve UI. It hosts three
// tabs — persisted editor history, persisted chart config history, and the
// in-memory query history (distinct runs and their cached results). It's opened
// from either the SQL editor or the chart config editor; the triggering button
// selects the initial tab.
export function HistoryModal({
  initialTab,
  onClose,
  onApplyEditor,
  onAppendEditor,
  onOpenRun,
  onRunsEvicted,
  onApplyConfig,
  chartData,
  chartLoading,
  chartError,
}: Props) {
  const [tab, setTab] = useState<HistoryTab>(initialTab);

  return (
    <Modal onClose={onClose} className="h-[80vh] w-[min(1600px,92vw)]">
      <div className="flex items-center justify-between border-b border-slate-200 px-2 py-1.5">
        <div className="flex items-center gap-1">
          {TABS.map(({ id, label }) => (
            <button
              key={id}
              type="button"
              onClick={() => setTab(id)}
              className={`rounded px-3 py-1 text-sm font-medium ${
                tab === id
                  ? 'bg-slate-100 text-slate-900'
                  : 'text-slate-500 hover:bg-slate-50 hover:text-slate-700'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700"
          aria-label="Close"
        >
          ✕
        </button>
      </div>

      {tab === 'editor' ? (
        <EditorHistoryPanel onApply={onApplyEditor} onAppend={onAppendEditor} />
      ) : null}
      {tab === 'query' ? (
        <QueryHistoryPanel onOpen={onOpenRun} onRunsEvicted={onRunsEvicted} />
      ) : null}
      {tab === 'chart' ? (
        <ChartHistoryPanel
          onApply={onApplyConfig}
          data={chartData}
          loading={chartLoading}
          dataError={chartError}
        />
      ) : null}
    </Modal>
  );
}
