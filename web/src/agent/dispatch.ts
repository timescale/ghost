import type { ResultView } from '../components/chart/types';
import { awaitExecutor, getExecutor } from './executor';
import { rowsToMatrix } from './runData';
import { renderChartImage } from './screenshot';
import type {
  AgentColumn,
  ChartCommand,
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
  getLastRunId(): string | null;
}

function toColumns(columns: { name: string; type?: string }[]): AgentColumn[] {
  return columns.map((c) => ({ name: c.name, type: c.type }));
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
  const result: VisualizeResult = {
    runId: outcome.runId,
    columns,
    rows: rowsToMatrix(data.rows, data.columns),
    rowCount: data.rows.length,
  };

  if (cmd.view === 'chart') {
    const config = cmd.chartConfig || deps.getState().chartConfig;
    result.image = await renderChartImage(config, data);
  }
  return result;
}

// handleChart reapplies a chart config to the last run and re-renders it.
async function handleChart(
  cmd: ChartCommand,
  deps: DispatchDeps,
): Promise<ChartResultWire> {
  deps.setChartConfig(cmd.chartConfig);
  deps.setResultView('chart');

  const runId = deps.getLastRunId();
  if (!runId) {
    throw new Error(
      'no completed query run to chart; run a query first (e.g. ghost_sql with visualize)',
    );
  }
  const executor = getExecutor();
  if (!executor) {
    throw new Error('no database panel is mounted to read results from');
  }
  // Read the full result for charting (the chart caps internally).
  const data = await executor.getRunData(runId, 50_000);
  const image = await renderChartImage(cmd.chartConfig, data);
  return { image };
}

interface ChartResultWire {
  image: string;
}

// handleUIState reads the current UI state plus the last run's results.
async function handleUIState(
  cmd: UIStateCommand,
  deps: DispatchDeps,
  getLastRun: () => {
    runId: string;
    status: string;
    rowCount: number;
    error?: string;
  } | null,
): Promise<UIStateResult> {
  const state = deps.getState();
  const result: UIStateResult = {
    selectedDatabaseId: state.selectedDatabaseId ?? undefined,
    editorSql: state.editorSql,
    chartConfig: state.chartConfig,
    resultView: state.resultView,
  };

  const lastRun = getLastRun();
  if (lastRun) {
    result.lastRun = {
      runId: lastRun.runId,
      status: lastRun.status,
      rowCount: lastRun.rowCount,
      error: lastRun.error,
    };
    const executor = getExecutor();
    if (executor && lastRun.status === 'success') {
      try {
        const data = await executor.getRunData(lastRun.runId, cmd.limit);
        result.lastRun.columns = toColumns(data.columns);
        result.lastRun.rows = rowsToMatrix(data.rows, data.columns);
        result.lastRun.rowCount = data.rows.length;
        // Include a chart image only when the chart view is currently visible.
        if (state.resultView === 'chart') {
          result.image = await renderChartImage(state.chartConfig, data);
        }
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
  getLastRun: () => {
    runId: string;
    status: string;
    rowCount: number;
    error?: string;
  } | null,
): Promise<unknown> {
  switch (type) {
    case 'visualize':
      return handleVisualize(payload as VisualizeCommand, deps);
    case 'chart':
      return handleChart(payload as ChartCommand, deps);
    case 'uiState':
      return handleUIState(payload as UIStateCommand, deps, getLastRun);
    default:
      throw new Error(`unknown command type: ${type}`);
  }
}
