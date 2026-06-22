import type { ChartData, ResultView } from '../components/chart/types';
import { tryGetChartConfigDiagnostics } from './diagnostics';
import { awaitExecutor, getExecutor } from './executor';
import { rowsToMatrix } from './runData';
import { renderChartImage } from './screenshot';
import type { AgentLastRun } from './store';
import type {
  AgentColumn,
  ChartCommand,
  ChartConfigDiagnostic,
  UIStateCommand,
  UIStateResult,
  VisualizeCommand,
  VisualizeResult,
} from './types';

// How long to wait for the target database's QueryPanel to mount after the
// agent switches the selection (it remounts on database change).
const EXECUTOR_WAIT_MS = 15_000;

// DispatchDeps is everything the dispatcher needs from the app: store
// accessors/mutators and a way to resolve a database ref (name or id) to its id.
export interface DispatchDeps {
  resolveDatabaseId(ref: string): string | null;
  getState(): {
    selectedDatabaseId: string | null;
    editorSql: string;
    chartConfig: string;
    resultView: ResultView;
  };
  setSelectedDatabaseId(id: string): void;
  setEditorSql(sql: string): void;
  setResultView(view: ResultView): void;
  setChartConfig(config: string): void;
  // The most recent query run in this tab (any database), or null. Handlers
  // must check its databaseId against the currently-mounted executor before
  // using it, so a run from a previously-selected database isn't read or
  // charted through the wrong panel.
  getLastRun(): AgentLastRun | null;
}

function toColumns(columns: { name: string; type?: string }[]): AgentColumn[] {
  return columns.map((c) => ({ name: c.name, type: c.type }));
}

// tryRenderChart renders a chart image and collects the config's editor
// diagnostics, returning either the image data URL or a render error message,
// plus any type/syntax diagnostics. It never throws: a bad chart config or
// unplottable data shouldn't fail the whole tool call, since the run data is
// still useful. Diagnostics are gathered even on a successful render, because
// many type errors (e.g. a misspelled option key) don't throw at runtime but
// still produce a wrong chart — surfacing them gives the agent the same
// feedback a human sees as red squiggles in the editor.
async function tryRenderChart(
  config: string,
  data: ChartData,
): Promise<{
  image?: string;
  chartError?: string;
  chartDiagnostics?: ChartConfigDiagnostic[];
}> {
  const [render, diagnostics] = await Promise.all([
    renderChartImage(config, data).then(
      (image) => ({ image }),
      (err) => ({
        chartError: err instanceof Error ? err.message : String(err),
      }),
    ),
    tryGetChartConfigDiagnostics(config),
  ]);
  return {
    ...render,
    chartDiagnostics: diagnostics.length > 0 ? diagnostics : undefined,
  };
}

// handleVisualize runs a query in the browser, syncing the live UI, and (for
// the chart view) renders a screenshot of the result.
async function handleVisualize(
  cmd: VisualizeCommand,
  deps: DispatchDeps,
): Promise<VisualizeResult> {
  // Trust the agent-supplied database ref instead of validating it against the
  // loaded database list: resolve it to a known id when the list is already
  // loaded (for a tidy UI selection), otherwise pass the raw ref through. The
  // frontend neither validates the ref nor waits for the list to load — the
  // backend resolves it (by id or name) when the query runs and surfaces any
  // invalid ref as a real error.
  const databaseId = deps.resolveDatabaseId(cmd.databaseRef) ?? cmd.databaseRef;

  // Sync the UI: select the database, set the editor SQL, and switch views.
  if (deps.getState().selectedDatabaseId !== databaseId) {
    deps.setSelectedDatabaseId(databaseId);
  }
  deps.setEditorSql(cmd.sql);
  if (cmd.chartConfig) deps.setChartConfig(cmd.chartConfig);
  deps.setResultView(cmd.view);

  const executor = await awaitExecutor(databaseId, EXECUTOR_WAIT_MS);
  const outcome = await executor.runQuery(cmd.sql);
  if (outcome.status === 'failed') {
    throw new Error(outcome.error || 'query failed');
  }

  const data = await executor.getRunData(outcome.runId, cmd.limit);
  const columns = toColumns(data.columns);

  // Always render a chart image so the agent can inspect the data visually,
  // regardless of which view the user is looking at — the screenshot is drawn
  // off-screen and doesn't depend on the visible pane. A render failure (e.g. a
  // bad chart config, or data the config can't plot) never fails the call: it's
  // reported as chartError alongside the run data.
  const config = cmd.chartConfig || deps.getState().chartConfig;
  const { image, chartError, chartDiagnostics } = await tryRenderChart(
    config,
    data,
  );

  return {
    runId: outcome.runId,
    columns,
    rows: rowsToMatrix(data.rows, data.columns),
    // Report the true total row count from the run, not the capped number of
    // rows read back — otherwise a query returning more than `limit` rows would
    // be reported (and summarized) as only `limit` rows, hiding truncation.
    rowCount: outcome.rowCount,
    image,
    chartError,
    chartDiagnostics,
  };
}

// handleChart reapplies a chart config to the last run and re-renders it.
async function handleChart(
  cmd: ChartCommand,
  deps: DispatchDeps,
): Promise<ChartResultWire> {
  deps.setChartConfig(cmd.chartConfig);
  deps.setResultView('chart');

  const executor = getExecutor();
  if (!executor) {
    throw new Error('no database panel is mounted to read results from');
  }
  // Only chart a run that belongs to the currently-mounted database. A run
  // recorded against a different database (before a database switch) must not be
  // read through this executor.
  const lastRun = deps.getLastRun();
  if (!lastRun || lastRun.databaseId !== executor.databaseId) {
    throw new Error(
      'no completed query run to chart for the current database; run a query first (e.g. ghost_sql with visualize)',
    );
  }
  // Read the full result for charting (the chart caps internally).
  const data = await executor.getRunData(lastRun.runId, 50_000);
  const [image, diagnostics] = await Promise.all([
    renderChartImage(cmd.chartConfig, data),
    tryGetChartConfigDiagnostics(cmd.chartConfig),
  ]);
  return {
    image,
    chartDiagnostics: diagnostics.length > 0 ? diagnostics : undefined,
  };
}

interface ChartResultWire {
  image: string;
  chartDiagnostics?: ChartConfigDiagnostic[];
}

// handleUIState reads the current UI state plus the last run's results.
async function handleUIState(
  cmd: UIStateCommand,
  deps: DispatchDeps,
): Promise<UIStateResult> {
  const state = deps.getState();
  const result: UIStateResult = {
    selectedDatabaseId: state.selectedDatabaseId ?? undefined,
    editorSql: state.editorSql,
    chartConfig: state.chartConfig,
    resultView: state.resultView,
  };

  const lastRun = deps.getLastRun();
  if (lastRun) {
    result.lastRun = {
      runId: lastRun.runId,
      status: lastRun.status,
      rowCount: lastRun.rowCount,
      error: lastRun.error,
    };
    const executor = getExecutor();
    // Only read back results for a run that belongs to the currently-mounted
    // database. After a database switch, the recorded last run may be from the
    // previous database; its results aren't available through this executor, so
    // we report the run metadata but skip the (mismatched) data/chart.
    if (
      executor &&
      lastRun.status === 'success' &&
      lastRun.databaseId === executor.databaseId
    ) {
      try {
        const data = await executor.getRunData(lastRun.runId, cmd.limit);
        result.lastRun.columns = toColumns(data.columns);
        result.lastRun.rows = rowsToMatrix(data.rows, data.columns);
        // Keep lastRun.rowCount as the true total reported on completion; do NOT
        // overwrite it with the capped number of rows read back here, or
        // truncation would be hidden from the agent.
        // Always render a chart image of the last run (off-screen, independent
        // of the visible view) so the agent can inspect it visually. A render
        // failure is reported as chartError rather than failing the call.
        const rendered = await tryRenderChart(state.chartConfig, data);
        result.image = rendered.image;
        result.chartError = rendered.chartError;
        result.chartDiagnostics = rendered.chartDiagnostics;
      } catch {
        // Best effort: if results can't be read, return the state without them.
      }
    }
  }
  return result;
}

// dispatch routes a command to its handler and returns the JSON-serializable
// result the server will deliver to the MCP tool.
export async function dispatch(
  type: string,
  payload: unknown,
  deps: DispatchDeps,
): Promise<unknown> {
  switch (type) {
    case 'visualize':
      return handleVisualize(payload as VisualizeCommand, deps);
    case 'chart':
      return handleChart(payload as ChartCommand, deps);
    case 'uiState':
      return handleUIState(payload as UIStateCommand, deps);
    default:
      throw new Error(`unknown command type: ${type}`);
  }
}
