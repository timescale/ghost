import {
  type ExecuteQueryData,
  ExecuteQueryEngine,
  type GetExecuteQueryDataArgs,
  QueryWidget,
} from '@timescale/popsql-query-widget-cdn';
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react';

import type { QueryHistoryEntry } from '../store';
import { formatAbsoluteTime, formatRelativeTime } from '../util/time';
import { ChartArea } from './chart/ChartArea';
import { ResultViewToggle } from './chart/ResultViewToggle';
import type { ResultView } from './chart/types';
import { useChartData } from './chart/useChartData';
import { WIDGET_REFERENCE_ID } from './widgetReference';

interface Props {
  entry: QueryHistoryEntry;
  // Make this run the active run in the main view, carrying over the result
  // view and (possibly tweaked) chart config the user is previewing with.
  onOpen: (entry: QueryHistoryEntry, view: ResultView, config: string) => void;
}

// The recorder is a no-op here: the query-history detail is a read-only preview,
// so tweaking its chart config must not pollute the chart-config history.
const noop = () => {};

// QueryHistoryDetail shows a single past run using the query widget's own
// read-only editor + results grid (keyed by the run's cached runId), so it
// matches the main view exactly. A view toggle switches between the table, the
// rendered chart, and the chart-config editor (seeded with the config captured
// at run time). The database, time, toggle, and "Open in editor" action live in
// the widget's toolbar. It must render inside the main results-cache provider so
// the grid reads the same cache the run executed in.
export function QueryHistoryDetail({ entry, onOpen }: Props) {
  // A unique widget id: the widget's QueryWidgetProvider is hosted at the app
  // root (see WidgetProviders), so every mounted editor needs an id unique
  // across the whole tree. Strip the colons React's useId adds so the id is
  // safe in the widget's editor model URI.
  const widgetId = `query-history-detail-${useId().replace(/[^a-z0-9]/gi, '')}`;
  // Local, ephemeral preview state. Editing the config here only affects this
  // preview until the user opens the run into the main editor.
  const [view, setView] = useState<ResultView>('table');
  const [config, setConfig] = useState(entry.chartConfig);
  const showTable = view === 'table';
  // Controlled editor height so it stays stable across view switches. In
  // 'split' mode the widget also fires onResizeEditor programmatically on any
  // container reflow (e.g. when the results grid appears/disappears switching
  // between table and chart), which would otherwise shrink the editor; only
  // honor height changes from a real user drag.
  const [editorHeight, setEditorHeight] = useState(160);
  // Ephemeral config-editor pane width: local to this preview so resizing it
  // doesn't mutate (or persist) the main view's chart layout.
  const [chartEditorWidth, setChartEditorWidth] = useState(480);
  const isResizingEditor = useRef(false);

  // Re-seed the preview when the selected run changes. The widget itself isn't
  // remounted (its runId prop drives the grid), so this keeps the local view
  // and config in step with the newly selected run.
  // biome-ignore lint/correctness/useExhaustiveDependencies: reset only when the run changes, not on every config edit
  useEffect(() => {
    setView('table');
    setConfig(entry.chartConfig);
  }, [entry.runId]);

  const { data, loading, error } = useChartData(entry.runId);

  // Required by QueryWidget but never invoked: the editor is read-only and the
  // run button hidden, so no query is executed — the grid renders the run's
  // already-cached results (by runId).
  const getExecuteQueryData = useCallback(
    ({ runId }: GetExecuteQueryDataArgs): ExecuteQueryData => ({
      engine: ExecuteQueryEngine.timescaleQuery,
      params: { projectId: '', serviceId: '', query: entry.sql, runId },
    }),
    [entry.sql],
  );

  // Capture "now" once per mount so all relative times share a single
  // reference (and renderToolbarAppendLeft's memoization holds).
  const now = useMemo(() => Date.now(), []);

  // Only the database name and run time — the run status and row count are
  // already shown by the widget's own status indicator and results grid, so
  // repeating them here would be redundant.
  const renderToolbarAppendLeft = useCallback(
    () => (
      <div className="flex flex-auto items-center gap-1.5 text-xs text-slate-500">
        <span className="font-medium text-slate-600">{entry.databaseName}</span>
        <span>·</span>
        <span title={formatAbsoluteTime(entry.ts)}>
          {formatRelativeTime(entry.ts, now)}
        </span>
      </div>
    ),
    [entry.databaseName, entry.ts, now],
  );

  const renderToolbarAppendRight = useCallback(
    () => (
      <div className="flex items-center gap-2">
        <ResultViewToggle value={view} onChange={setView} />
        <button
          type="button"
          onClick={() => onOpen(entry, view, config)}
          className="rounded border border-slate-800 bg-slate-800 px-2 py-1 text-xs text-white hover:bg-slate-700"
        >
          Open in editor
        </button>
      </div>
    ),
    [view, config, entry, onOpen],
  );

  return (
    <div className="flex min-w-0 flex-1 flex-col p-2">
      <QueryWidget
        id={widgetId}
        className={showTable ? 'flex-auto' : undefined}
        query={entry.sql}
        runId={entry.runId}
        referenceId={WIDGET_REFERENCE_ID}
        getExecuteQueryData={getExecuteQueryData}
        readonlyEditor
        disableRun
        hideRunButton
        hideSessionStatus
        hideResults={!showTable}
        resizeHandles={showTable ? 'split' : 'editor'}
        editorMinHeight={120}
        editorHeight={editorHeight}
        onResizeStart={() => {
          isResizingEditor.current = true;
        }}
        onResizeStop={() => {
          isResizingEditor.current = false;
        }}
        onResizeEditor={(height) => {
          if (isResizingEditor.current) setEditorHeight(height);
        }}
        renderToolbarAppendLeft={renderToolbarAppendLeft}
        renderToolbarAppendRight={renderToolbarAppendRight}
      />
      {showTable ? null : (
        <ChartArea
          view={view}
          config={config}
          onConfigChange={setConfig}
          data={data}
          loading={loading}
          error={error}
          onRenderSuccess={noop}
          editorWidth={chartEditorWidth}
          onEditorWidthChange={setChartEditorWidth}
        />
      )}
    </div>
  );
}
