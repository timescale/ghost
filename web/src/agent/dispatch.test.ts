import { afterEach, beforeEach, describe, expect, mock, test } from 'bun:test';

import type { ResultView } from '../components/chart/types';
import type { DispatchDeps } from './dispatch';
import { dispatch } from './dispatch';
import type { Executor } from './executor';
import { registerExecutor } from './executor';
import type { AgentLastRun } from './store';
import type {
  ChartResult,
  UIStateResult,
  VisualizeCommand,
  VisualizeResult,
} from './types';

// Stub the diagnostics module: it loads Monaco from a CDN, which can't run in
// the test environment. The dispatcher's diagnostics collection is best-effort
// and tested separately (flattenMessage.test.ts); here we only care that it
// doesn't block or fail the dispatch path. Bun hoists mock.module so this takes
// effect for the static dispatch import above.
mock.module('./diagnostics', () => ({
  tryGetChartConfigDiagnostics: async () => [],
}));

// A never-aborted signal for dispatch calls in tests that don't exercise
// cancellation.
const noSignal = (): AbortSignal => new AbortController().signal;

// makeDeps builds a DispatchDeps whose resolveDatabaseId mimics the app: it
// returns the id for refs in the (possibly empty) known list, else null. It
// records which database id the dispatcher actually selected, plus the chart
// config and result views it set (so tests can assert no UI mutation on error).
function makeDeps(known: string[]): {
  deps: DispatchDeps;
  selected: string[];
  editorSql: () => string;
  chartConfig: () => string;
  resultViews: ResultView[];
} {
  const selected: string[] = [];
  let editorSql = '';
  let chartConfig = '';
  const resultViews: ResultView[] = [];
  const deps: DispatchDeps = {
    resolveDatabaseId: (ref) => (known.includes(ref) ? ref : null),
    getState: () => ({
      selectedDatabaseId: selected.at(-1) ?? null,
      editorSql,
      chartConfig,
      resultView: resultViews.at(-1) ?? ('table' as ResultView),
    }),
    setSelectedDatabaseId: (id) => {
      selected.push(id);
    },
    setEditorSql: (sql) => {
      editorSql = sql;
    },
    setResultView: (view) => {
      resultViews.push(view);
    },
    setChartConfig: (config) => {
      chartConfig = config;
    },
    getLastRun: () => null,
  };
  return {
    deps,
    selected,
    editorSql: () => editorSql,
    chartConfig: () => chartConfig,
    resultViews,
  };
}

// makeResetExecutor builds a no-op executor used to clear the registry between
// tests (registering it then immediately unregistering).
function makeResetExecutor(): Executor {
  return {
    databaseId: 'reset',
    runQuery: async () => ({
      runId: 'r',
      status: 'success' as const,
      rowCount: 0,
      rowsAffected: 0,
    }),
    getRunData: async () => ({ rows: [], columns: [] }),
  };
}

function clearRegistry(): void {
  const cleanup = registerExecutor(makeResetExecutor());
  cleanup();
}

// registerStubExecutor installs a stub executor. totalRowCount is the total the
// run reports on completion; getRunData honors its limit argument (returning
// min(limit, totalRowCount) rows) like the real results-cache read, so a caller
// that under-reads the data shows up as too few rows. rowsAffected is the
// Postgres command-tag count the run reports, defaulting to totalRowCount. The
// returned `getRunDataLimits` records every limit getRunData was called with, so
// tests can assert that charting reads the full result rather than the agent's
// (possibly smaller) row cap.
function registerStubExecutor(
  databaseId: string,
  totalRowCount = 1,
  rowsAffected = totalRowCount,
): { getRunDataLimits: number[] } {
  const getRunDataLimits: number[] = [];
  const executor: Executor = {
    databaseId,
    runQuery: async () => ({
      runId: 'run-1',
      status: 'success' as const,
      rowCount: totalRowCount,
      rowsAffected,
    }),
    getRunData: async (_runId, limit) => {
      getRunDataLimits.push(limit);
      const count = Math.min(limit, totalRowCount);
      return {
        rows: Array.from({ length: count }, (_, i) => ({ n: i + 1 })),
        columns: [{ name: 'n', type: 'INT8' }],
      };
    },
  };
  registerExecutor(executor);
  return { getRunDataLimits };
}

const visualizeCmd = (databaseRef: string): VisualizeCommand => ({
  databaseRef,
  sql: 'SELECT 1 AS n',
  limit: 50,
});

describe('dispatch visualize', () => {
  beforeEach(clearRegistry);
  afterEach(clearRegistry);

  test('resolves a known ref to its id and selects it', async () => {
    const { deps, selected } = makeDeps(['db1']);
    registerStubExecutor('db1');
    const result = (await dispatch(
      'visualize',
      visualizeCmd('db1'),
      deps,
      noSignal(),
    )) as VisualizeResult;
    expect(selected).toEqual(['db1']);
    expect(result.runId).toBe('run-1');
    expect(result.rowCount).toBe(1);
  });

  test('reports the true total row count, not the capped number read', async () => {
    // The run produced 10,000 rows but the command's limit is 50. The result
    // must report the total (10,000), not the capped read (50), so the agent
    // knows the output was truncated.
    const { deps } = makeDeps(['db1']);
    registerStubExecutor('db1', 10_000);
    const result = (await dispatch(
      'visualize',
      visualizeCmd('db1'),
      deps,
      noSignal(),
    )) as VisualizeResult;
    expect(result.rowCount).toBe(10_000);
    expect(result.rows.length).toBe(50);
  });

  test('charts the full result set even when the agent caps returned rows', async () => {
    // Regression: the run produced 59 rows but the agent requested only 5 (to
    // keep its context small). The 5-row cap must apply ONLY to the rows
    // returned to the agent — the chart must still be fed all 59 rows, or the
    // rendered image shows just the first 5 points. The chart read must request
    // far more than the agent's row cap.
    const { deps } = makeDeps(['db1']);
    const { getRunDataLimits } = registerStubExecutor('db1', 59);
    const result = (await dispatch(
      'visualize',
      { ...visualizeCmd('db1'), limit: 5 },
      deps,
      noSignal(),
    )) as VisualizeResult;
    // The agent gets only its requested 5 rows back...
    expect(result.rows.length).toBe(5);
    expect(result.rowCount).toBe(59);
    // ...but the chart was fed the full result set, not the 5-row cap.
    expect(Math.max(...getRunDataLimits)).toBeGreaterThanOrEqual(59);
  });

  test('carries the command-tag rowsAffected through to the result', async () => {
    // The run's command-tag count (e.g. rows touched by a DELETE) is reported
    // separately from the total row count, and must reach the result so the
    // structured tool output's rows_affected is accurate.
    const { deps } = makeDeps(['db1']);
    registerStubExecutor('db1', 0, 7);
    const result = (await dispatch(
      'visualize',
      visualizeCmd('db1'),
      deps,
      noSignal(),
    )) as VisualizeResult;
    expect(result.rowsAffected).toBe(7);
  });

  test('forwards the abort signal to the executor run', async () => {
    // The visualize handler must pass its command's AbortSignal down to
    // runQuery so a canceled MCP request aborts this run's query (and only it).
    const { deps } = makeDeps(['db1']);
    let seenSignal: AbortSignal | undefined;
    const executor: Executor = {
      databaseId: 'db1',
      runQuery: async (_sql, signal) => {
        seenSignal = signal;
        return {
          runId: 'run-1',
          status: 'success' as const,
          rowCount: 1,
          rowsAffected: 1,
        };
      },
      getRunData: async () => ({
        rows: [{ n: 1 }],
        columns: [{ name: 'n', type: 'INT8' }],
      }),
    };
    registerExecutor(executor);
    const controller = new AbortController();
    await dispatch('visualize', visualizeCmd('db1'), deps, controller.signal);
    expect(seenSignal).toBe(controller.signal);
  });

  test('reports a chart render failure as chartError but still returns rows', async () => {
    // ECharts isn't loaded in the test environment, so rendering fails. With a
    // chart_config supplied, the dispatcher must surface that as chartError
    // without failing the call or dropping the run data.
    const { deps } = makeDeps(['db1']);
    registerStubExecutor('db1');
    const result = (await dispatch(
      'visualize',
      { ...visualizeCmd('db1'), chartConfig: 'function chart(){ return {}; }' },
      deps,
      noSignal(),
    )) as VisualizeResult;
    expect(result.runId).toBe('run-1');
    expect(result.rowCount).toBe(1);
    expect(result.image).toBeUndefined();
    expect(result.chartError).toBeTruthy();
  });

  test('renders no chart when no chart_config is provided', async () => {
    // Without a chart_config the agent didn't ask to chart, so the result
    // carries only the rows — no image, chartError, or diagnostics — rather than
    // a surprise default-config chart.
    const { deps, resultViews } = makeDeps(['db1']);
    registerStubExecutor('db1');
    const result = (await dispatch(
      'visualize',
      visualizeCmd('db1'),
      deps,
      noSignal(),
    )) as VisualizeResult;
    expect(result.rowCount).toBe(1);
    expect(result.image).toBeUndefined();
    expect(result.chartError).toBeUndefined();
    expect(result.chartDiagnostics).toBeUndefined();
    // The active view is left unchanged when no config is given.
    expect(resultViews).toEqual([]);
  });

  test('switches to the chart view when a chart_config is provided', async () => {
    // Supplying a chart_config is the signal to chart: apply the config and
    // switch the live UI to the chart view.
    const { deps, chartConfig, resultViews } = makeDeps(['db1']);
    registerStubExecutor('db1');
    await dispatch(
      'visualize',
      { ...visualizeCmd('db1'), chartConfig: 'function chart(){ return {}; }' },
      deps,
      noSignal(),
    );
    expect(chartConfig()).toBe('function chart(){ return {}; }');
    expect(resultViews).toEqual(['chart']);
  });

  test('passes an unresolved ref through without throwing (list not loaded)', async () => {
    // Empty known list simulates /api/databases not having loaded yet. The
    // dispatcher must trust the agent-supplied ref rather than reject it.
    const { deps, selected } = makeDeps([]);
    registerStubExecutor('db-unlisted');
    const result = (await dispatch(
      'visualize',
      visualizeCmd('db-unlisted'),
      deps,
      noSignal(),
    )) as VisualizeResult;
    expect(selected).toEqual(['db-unlisted']);
    expect(result.runId).toBe('run-1');
  });
});

const stubLastRun = (databaseId: string): AgentLastRun => ({
  databaseId,
  runId: 'run-1',
  status: 'success',
  rowCount: 1,
  rowsAffected: 1,
});

describe('dispatch chart', () => {
  afterEach(clearRegistry);

  test('throws when no executor is mounted', async () => {
    const { deps } = makeDeps(['db1']);
    await expect(
      dispatch(
        'chart',
        { chartConfig: 'function chart(){}' },
        deps,
        noSignal(),
      ),
    ).rejects.toThrow('no database panel is mounted');
  });

  test('throws when the last run belongs to a different database', async () => {
    const { deps } = makeDeps(['db1']);
    registerStubExecutor('db1');
    deps.getLastRun = () => stubLastRun('db2');
    await expect(
      dispatch(
        'chart',
        { chartConfig: 'function chart(){}' },
        deps,
        noSignal(),
      ),
    ).rejects.toThrow('no completed query run to chart');
  });

  test('throws when the last run failed (no readable results)', async () => {
    // A failed last run is recorded for the current database but has no cached
    // results to chart; charting it must reject (and not mutate the UI), not
    // attempt a read of a run that produced nothing.
    const { deps, chartConfig, resultViews } = makeDeps(['db1']);
    registerStubExecutor('db1');
    deps.getLastRun = () => ({ ...stubLastRun('db1'), status: 'failed' });
    await expect(
      dispatch(
        'chart',
        { chartConfig: 'function chart(){}' },
        deps,
        noSignal(),
      ),
    ).rejects.toThrow('no completed query run to chart');
    expect(chartConfig()).toBe('');
    expect(resultViews).toEqual([]);
  });

  test('does not mutate the UI when validation fails', async () => {
    // A failed chart command (no matching run) must not clobber the user's
    // chart config or switch their view: validation happens before any mutation.
    const { deps, chartConfig, resultViews } = makeDeps(['db1']);
    registerStubExecutor('db1');
    deps.getLastRun = () => stubLastRun('db2');
    await expect(
      dispatch(
        'chart',
        { chartConfig: 'function chart(){}' },
        deps,
        noSignal(),
      ),
    ).rejects.toThrow('no completed query run to chart');
    expect(chartConfig()).toBe('');
    expect(resultViews).toEqual([]);
  });

  test('does not mutate the UI when the results read fails', async () => {
    // The run is a valid success for the current database, but its cached
    // results are gone (e.g. evicted), so getRunData rejects. The UI must be
    // left untouched — config/view are applied only after the read succeeds.
    const { deps, chartConfig, resultViews } = makeDeps(['db1']);
    const executor: Executor = {
      databaseId: 'db1',
      runQuery: async () => ({
        runId: 'run-1',
        status: 'success' as const,
        rowCount: 1,
        rowsAffected: 1,
      }),
      getRunData: async () => {
        throw new Error('results no longer cached');
      },
    };
    registerExecutor(executor);
    deps.getLastRun = () => stubLastRun('db1');
    await expect(
      dispatch(
        'chart',
        { chartConfig: 'function chart(){}' },
        deps,
        noSignal(),
      ),
    ).rejects.toThrow('results no longer cached');
    expect(chartConfig()).toBe('');
    expect(resultViews).toEqual([]);
  });

  test('does not mutate the UI when aborted during the results read', async () => {
    // The command is valid, but it's abandoned (canceled / takeover / SSE drop)
    // while getRunData is in flight. handleChart must observe the abort after
    // the read and reject without applying the config or switching the view —
    // otherwise an abandoned command clobbers the UI after its request failed.
    const { deps, chartConfig, resultViews } = makeDeps(['db1']);
    const controller = new AbortController();
    let resolveRead:
      | ((data: { rows: never[]; columns: never[] }) => void)
      | null = null;
    const executor: Executor = {
      databaseId: 'db1',
      runQuery: async () => ({
        runId: 'run-1',
        status: 'success' as const,
        rowCount: 1,
        rowsAffected: 1,
      }),
      getRunData: () =>
        new Promise((resolve) => {
          resolveRead = resolve;
        }),
    };
    registerExecutor(executor);
    deps.getLastRun = () => stubLastRun('db1');

    const promise = dispatch(
      'chart',
      { chartConfig: 'function chart(){}' },
      deps,
      controller.signal,
    );
    // Abort while the read is still pending, then let the read resolve.
    controller.abort();
    resolveRead?.({ rows: [], columns: [] });

    await expect(promise).rejects.toThrow('the chart command was canceled');
    expect(chartConfig()).toBe('');
    expect(resultViews).toEqual([]);
  });

  test('reports a render failure as chartError instead of throwing', async () => {
    // ECharts isn't loaded in the test environment, so rendering fails. Like the
    // visualize path, handleChart must surface that as chartError without
    // failing the call or dropping the editor diagnostics.
    const { deps } = makeDeps(['db1']);
    registerStubExecutor('db1');
    deps.getLastRun = () => stubLastRun('db1');
    const result = (await dispatch(
      'chart',
      { chartConfig: 'function chart(){}' },
      deps,
      noSignal(),
    )) as ChartResult;
    expect(result.image).toBeUndefined();
    expect(result.chartError).toBeTruthy();
  });
});

describe('dispatch uiState', () => {
  afterEach(clearRegistry);

  test('charts the full last-run result even when caps returned rows', async () => {
    // Regression: like the visualize path, ghost_ui_state must feed the chart
    // the full result set, not the agent's row cap. The last run produced 59
    // rows but the agent requested only 5; the chart read must request far more
    // than 5, while the returned rows stay capped at 5.
    const { deps } = makeDeps(['db1']);
    const { getRunDataLimits } = registerStubExecutor('db1', 59);
    deps.getLastRun = () => ({ ...stubLastRun('db1'), rowCount: 59 });
    const result = (await dispatch(
      'uiState',
      { limit: 5 },
      deps,
      noSignal(),
    )) as UIStateResult;
    expect(result.lastRun?.rows?.length).toBe(5);
    expect(result.lastRun?.rowCount).toBe(59);
    expect(Math.max(...getRunDataLimits)).toBeGreaterThanOrEqual(59);
  });
});
