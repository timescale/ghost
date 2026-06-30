import { useCallback, useState } from 'react';

import { useServeStore } from '../../store';
import { Icon } from '../Icon';
import { SplitPane } from '../SplitPane';
import { ChartConfigEditor } from './ChartConfigEditor';
import { ChartHistoryModal } from './ChartHistoryModal';
import { ChartView } from './ChartView';
import type { ResultView } from './types';
import { useChartConfigRecorder } from './useChartConfigRecorder';
import { useChartData } from './useChartData';

interface Props {
  // The runId of the most recent successful query run, or null if none yet.
  runId: string | null;
  // 'chart_editor' shows the config editor beside a live preview; any other value
  // ('chart') renders the chart full-bleed. ('table' is handled upstream.)
  view: ResultView;
  config: string;
  onConfigChange: (next: string) => void;
}

// ChartArea fills the space below the query editor when the chart or editor
// view is active. It must be rendered inside the widget's
// ResultsCacheContext so useChartData can read the cached results.
export function ChartArea({ runId, view, config, onConfigChange }: Props) {
  const { data, loading, error } = useChartData(runId);
  const chartEditorWidth = useServeStore((s) => s.chartEditorWidth);
  const setChartEditorWidth = useServeStore((s) => s.setChartEditorWidth);
  // Seed the recorder's baseline with the config loaded at mount, so the first
  // user-authored edit is recorded even if it happens before the first render.
  const { recordRenderSuccess, markApplied } = useChartConfigRecorder(config);
  const [historyOpen, setHistoryOpen] = useState(false);

  // Applying a config from history: mark it so the recorder doesn't treat it as
  // a fresh user edit, then push it into the editor and close the modal.
  const handleApply = useCallback(
    (next: string) => {
      markApplied(next);
      onConfigChange(next);
      setHistoryOpen(false);
    },
    [markApplied, onConfigChange],
  );

  const editorPane = (
    <div className="flex min-h-0 flex-auto flex-col">
      <div className="flex items-center gap-2 border-b border-slate-200 px-2 py-1.5">
        <button
          type="button"
          onClick={() => setHistoryOpen(true)}
          aria-label="Chart config history"
          title="Chart config history"
          className="rounded border border-slate-300 bg-white p-1 text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-800"
        >
          <Icon name="history" size="sm" color="current" />
        </button>
        <span className="text-xs font-medium text-slate-500">Chart config</span>
      </div>
      <div className="min-h-0 flex-auto">
        <ChartConfigEditor config={config} onChange={onConfigChange} />
      </div>
    </div>
  );

  return (
    <>
      <SplitPane
        className="mt-4 flex flex-auto overflow-hidden rounded-lg border border-slate-200 bg-white"
        showLeft={view === 'chart_editor'}
        leftWidth={chartEditorWidth}
        minLeftWidth={280}
        minRightWidth={300}
        onLeftWidthChange={setChartEditorWidth}
        left={editorPane}
        right={
          <ChartView
            data={data}
            loading={loading}
            dataError={error}
            config={config}
            onRenderSuccess={recordRenderSuccess}
          />
        }
      />
      {historyOpen ? (
        <ChartHistoryModal
          onClose={() => setHistoryOpen(false)}
          onApply={handleApply}
          data={data}
          loading={loading}
          dataError={error}
        />
      ) : null}
    </>
  );
}
