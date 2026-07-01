import { SplitPane } from '../SplitPane';
import { ChartConfigEditor } from './ChartConfigEditor';
import { ChartView } from './ChartView';
import type { ChartData, ResultView } from './types';

interface Props {
  // 'chart_editor' shows the config editor beside a live preview; any other value
  // ('chart') renders the chart full-bleed. ('table' is handled upstream.)
  view: ResultView;
  config: string;
  onConfigChange: (next: string) => void;
  // Cached results for the active run, resolved by the parent (QueryPanel).
  data: ChartData | null;
  loading: boolean;
  error: string | null;
  // Called when a config renders cleanly, so the parent's recorder can capture
  // user-authored configs into history.
  onRenderSuccess: (config: string) => void;
  // Width (px) of the config editor pane and a setter for it. The parent owns
  // this: the main view persists it to the store, while the read-only query-
  // history preview keeps it in ephemeral local state so resizing the preview
  // doesn't mutate (and persist) the main layout.
  editorWidth: number;
  onEditorWidthChange: (
    width: number | ((prevWidth: number) => number),
  ) => void;
}

// ChartArea fills the space below the query editor when the chart or editor
// view is active. It's presentational: the parent (QueryPanel) owns the run's
// cached results and the chart-config history modal.
export function ChartArea({
  view,
  config,
  onConfigChange,
  data,
  loading,
  error,
  onRenderSuccess,
  editorWidth,
  onEditorWidthChange,
}: Props) {
  const editorPane = (
    <div className="flex min-h-0 flex-auto flex-col">
      <div className="flex items-center gap-2 border-b border-slate-200 px-2 py-1.5">
        <span className="text-xs font-medium text-slate-500">Chart config</span>
      </div>
      <div className="min-h-0 flex-auto">
        <ChartConfigEditor config={config} onChange={onConfigChange} />
      </div>
    </div>
  );

  return (
    <SplitPane
      className="mt-4 flex flex-auto overflow-hidden rounded-lg border border-slate-200 bg-white"
      showLeft={view === 'chart_editor'}
      leftWidth={editorWidth}
      minLeftWidth={280}
      minRightWidth={300}
      onLeftWidthChange={onEditorWidthChange}
      left={editorPane}
      right={
        <ChartView
          data={data}
          loading={loading}
          dataError={error}
          config={config}
          onRenderSuccess={onRenderSuccess}
        />
      }
    />
  );
}
