import { useServeStore } from '../../store';
import { SplitPane } from '../SplitPane';
import { ChartConfigEditor } from './ChartConfigEditor';
import { ChartView } from './ChartView';
import type { ResultView } from './types';
import { useChartData } from './useChartData';

interface Props {
  // The runId of the most recent successful query run, or null if none yet.
  runId: string | null;
  // 'editor' shows the config editor beside a live preview; any other value
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

  return (
    <SplitPane
      className="mt-4 flex flex-auto overflow-hidden rounded-lg border border-slate-200 bg-white"
      showLeft={view === 'editor'}
      leftWidth={chartEditorWidth}
      minLeftWidth={280}
      minRightWidth={300}
      onLeftWidthChange={setChartEditorWidth}
      left={<ChartConfigEditor config={config} onChange={onConfigChange} />}
      right={
        <ChartView
          data={data}
          loading={loading}
          dataError={error}
          config={config}
        />
      }
    />
  );
}
