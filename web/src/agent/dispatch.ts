import { ensureChartTypeAnnotation } from '../components/chart/configHeader';
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

// Row cap applied when reading a run's results for charting. The agent's
// `limit` parameter bounds only the rows handed back to it (to keep its context
// small) — it must NOT bound what the chart is fed, or a small limit (e.g. 5)
// would render only the first few points of a larger result set. We read up to
// this many rows for the chart (the chart caps internally too); it mirrors the
// on-screen MAX_CHART_ROWS in useChartData so the screenshot matches the live UI.
const CHART_ROW_LIMIT = 50_000;

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

// handleVisualize runs a query in the browser, syncing the live UI, and renders
// a screenshot of the result (off-screen, regardless of the active view).
// `signal` aborts this command's own query (and only it) if the MCP request is
// canceled, times out, or is superseded.
async function handleVisualize(
  cmd: VisualizeCommand,
  deps: DispatchDeps,
  signal: AbortSignal,
): Promise<VisualizeResult> {
  // Trust the agent-supplied database ref instead of validating it against the
  // loaded database list: resolve it to a known id when the list is already
  // loaded (for a tidy UI selection), otherwise pass the raw ref through. The
  // frontend neither validates the ref nor waits for the list to load — the
  // backend resolves it (by id or name) when the query runs and surfaces any
  // invalid ref as a real error.
  const databaseId = deps.resolveDatabaseId(cmd.databaseRef) ?? cmd.databaseRef;

  // Sync the UI: select the database and set the editor SQL. When a chart
  // config is supplied, apply it and switch to the chart view so the user sees
  // it; otherwise leave the active view unchanged (the agent didn't ask to
  // chart, and a chart image is still returned to it off-screen regardless).
  if (deps.getState().selectedDatabaseId !== databaseId) {
    deps.setSelectedDatabaseId(databaseId);
  }
  deps.setEditorSql(cmd.sql);
  if (cmd.chartConfig) {
    // Store the annotated form so the live editor's model is fully typed
    // (hover, completions, squiggles); the raw source the agent sent is still
    // what gets rendered and diagnosed below, so the line numbers reported
    // back to the agent match the config it wrote.
    deps.setChartConfig(ensureChartTypeAnnotation(cmd.chartConfig));
    deps.setResultView('chart');
  }

  const executor = await awaitExecutor(databaseId, EXECUTOR_WAIT_MS, signal);
  const outcome = await executor.runQuery(cmd.sql, signal);
  if (outcome.status === 'failed') {
    throw new Error(outcome.error || 'query failed');
  }

  // Read the full result set (capped only at the chart's row limit), not the
  // agent's `limit`: the chart must be fed every row, while the agent's `limit`
  // caps only the rows returned in the response. Reading once and slicing for
  // the agent keeps the chart and the returned rows consistent and avoids a
  // second cache read.
  const data = await executor.getRunData(outcome.runId, CHART_ROW_LIMIT);
  const columns = toColumns(data.columns);

  // Render a chart image only when the agent supplied a chart_config — that's
  // its signal that it wants the data charted. Without one, return just the
  // rows (no image/chartError/diagnostics), so a plain query isn't surprised
  // with a default-config chart it never asked for. When a config is given, the
  // screenshot is drawn off-screen (independent of the visible pane), and a
  // render failure (bad config or unplottable data) never fails the call — it's
  // reported as chartError alongside the run data.
  const chart = cmd.chartConfig
    ? await tryRenderChart(cmd.chartConfig, data)
    : {};

  // Cap the rows handed back to the agent to its requested limit (the chart
  // above already rendered the full result set).
  const agentRows = data.rows.slice(0, cmd.limit);

  return {
    runId: outcome.runId,
    columns,
    rows: rowsToMatrix(agentRows, data.columns),
    // Report the true total row count from the run, not the capped number of
    // rows read back — otherwise a query returning more than `limit` rows would
    // be reported (and summarized) as only `limit` rows, hiding truncation.
    rowCount: outcome.rowCount,
    // The Postgres command-tag count, surfaced in the structured tool output's
    // rows_affected (matching the server-side ghost_sql path).
    rowsAffected: outcome.rowsAffected,
    image: chart.image,
    chartError: chart.chartError,
    chartDiagnostics: chart.chartDiagnostics,
  };
}

// handleChart reapplies a chart config to the last run and re-renders it.
// `signal` aborts the command if it's canceled, superseded, or the SSE stream
// drops — so an abandoned command can't clobber the UI after its request failed.
async function handleChart(
  cmd: ChartCommand,
  deps: DispatchDeps,
  signal: AbortSignal,
): Promise<ChartResultWire> {
  // Validate BEFORE mutating the UI: if there's no mounted executor or no
  // matching last run, this command fails, and it must not clobber the user's
  // chart config or switch their view as a side effect of an error.
  const executor = getExecutor();
  if (!executor) {
    throw new Error('no database panel is mounted to read results from');
  }
  // Only chart a successful run that belongs to the currently-mounted database.
  // A run recorded against a different database (before a database switch) must
  // not be read through this executor; a failed run has no readable results in
  // the cache, so charting it would mutate the UI and then error.
  const lastRun = deps.getLastRun();
  if (
    !lastRun ||
    lastRun.databaseId !== executor.databaseId ||
    lastRun.status !== 'success'
  ) {
    throw new Error(
      'no completed query run to chart for the current database; run a query first (e.g. ghost_visualize with sql)',
    );
  }

  // Read the run's results BEFORE mutating the UI: if the cache entry is gone
  // (e.g. evicted), this rejects, and we must not have clobbered the user's
  // chart config or switched their view as a side effect of a failed command.
  // Read the full result for charting (the chart caps internally).
  const data = await executor.getRunData(lastRun.runId, CHART_ROW_LIMIT);

  // The command may have been abandoned during the read (canceled, another tab
  // took over, or the SSE stream dropped — all of which abort the signal, and
  // the server has already failed the request). Bail before mutating so an
  // abandoned command can't overwrite the user's chart config or switch their
  // view. Also re-check the executor/run still match: a database switch during
  // the read would remount the panel, so applying now would target the wrong DB.
  if (signal.aborted) {
    throw new Error('the chart command was canceled');
  }
  const currentExecutor = getExecutor();
  const currentRun = deps.getLastRun();
  if (
    currentExecutor !== executor ||
    currentRun?.runId !== lastRun.runId ||
    currentRun?.databaseId !== lastRun.databaseId
  ) {
    throw new Error(
      'the active query run changed before the chart could apply',
    );
  }

  // Results are readable and still current: apply the config and switch view.
  // The annotated form is stored for the live editor (typed hover/completions);
  // the raw source is rendered and diagnosed below, so reported line numbers
  // match the config the agent wrote.
  deps.setChartConfig(ensureChartTypeAnnotation(cmd.chartConfig));
  deps.setResultView('chart');

  // Reuse tryRenderChart so a bad config doesn't fail the whole tool call:
  // it's reported as chartError alongside the editor diagnostics, matching the
  // ghost_sql visualize path. Many type errors don't throw at runtime but still
  // produce a wrong chart, so surfacing diagnostics even on a successful render
  // gives the agent the same feedback a human sees as red squiggles.
  const { image, chartError, chartDiagnostics } = await tryRenderChart(
    cmd.chartConfig,
    data,
  );
  return { image, chartError, chartDiagnostics };
}

interface ChartResultWire {
  image?: string;
  chartError?: string;
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
      rowsAffected: lastRun.rowsAffected,
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
        // Read the full result set (capped at the chart row limit) so the chart
        // renders every row; the agent's `limit` caps only the rows we return.
        const data = await executor.getRunData(lastRun.runId, CHART_ROW_LIMIT);
        result.lastRun.columns = toColumns(data.columns);
        result.lastRun.rows = rowsToMatrix(
          data.rows.slice(0, cmd.limit),
          data.columns,
        );
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
// result the server will deliver to the MCP tool. `signal` aborts the command
// if its request is abandoned: `visualize` wires it to cancel its in-flight
// query, and `chart` checks it before mutating the UI after its (async) results
// read. `uiState` is read-only and short, so it ignores the signal.
export async function dispatch(
  type: string,
  payload: unknown,
  deps: DispatchDeps,
  signal: AbortSignal,
): Promise<unknown> {
  switch (type) {
    case 'visualize':
      return handleVisualize(payload as VisualizeCommand, deps, signal);
    case 'chart':
      return handleChart(payload as ChartCommand, deps, signal);
    case 'uiState':
      return handleUIState(payload as UIStateCommand, deps);
    default:
      throw new Error(`unknown command type: ${type}`);
  }
}
