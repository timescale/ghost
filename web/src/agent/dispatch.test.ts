import { afterEach, beforeEach, describe, expect, mock, test } from 'bun:test';

import type { ResultView } from '../components/chart/types';
import type { DispatchDeps } from './dispatch';
import { dispatch } from './dispatch';
import type { Executor } from './executor';
import { registerExecutor } from './executor';
import type { VisualizeCommand, VisualizeResult } from './types';

// Stub the diagnostics module: it loads Monaco from a CDN, which can't run in
// the test environment. The dispatcher's diagnostics collection is best-effort
// and tested separately (flattenMessage.test.ts); here we only care that it
// doesn't block or fail the dispatch path. Bun hoists mock.module so this takes
// effect for the static dispatch import above.
mock.module('./diagnostics', () => ({
  tryGetChartConfigDiagnostics: async () => [],
}));

// makeDeps builds a DispatchDeps whose resolveDatabaseId mimics the app: it
// returns the id for refs in the (possibly empty) known list, else null. It
// records which database id the dispatcher actually selected.
function makeDeps(known: string[]): {
  deps: DispatchDeps;
  selected: string[];
  editorSql: () => string;
} {
  const selected: string[] = [];
  let editorSql = '';
  const deps: DispatchDeps = {
    resolveDatabaseId: (ref) => (known.includes(ref) ? ref : null),
    getState: () => ({
      selectedDatabaseId: selected.at(-1) ?? null,
      editorSql,
      chartConfig: '',
      resultView: 'table' as ResultView,
    }),
    setSelectedDatabaseId: (id) => {
      selected.push(id);
    },
    setEditorSql: (sql) => {
      editorSql = sql;
    },
    setResultView: () => {},
    setChartConfig: () => {},
    getLastRunId: () => null,
  };
  return { deps, selected, editorSql: () => editorSql };
}

// registerStubExecutor installs a stub executor. totalRowCount is the total the
// run reports on completion; rowsRead is the number of rows getRunData returns
// (the capped read), defaulting to totalRowCount when omitted.
function registerStubExecutor(
  databaseId: string,
  totalRowCount = 1,
  rowsRead = totalRowCount,
): void {
  const executor: Executor = {
    databaseId,
    runQuery: async () => ({
      runId: 'run-1',
      status: 'success' as const,
      rowCount: totalRowCount,
    }),
    getRunData: async () => ({
      rows: Array.from({ length: rowsRead }, (_, i) => ({ n: i + 1 })),
      columns: [{ name: 'n', type: 'INT8' }],
    }),
    cancelQuery: () => {},
  };
  registerExecutor(executor);
}

const visualizeCmd = (databaseRef: string): VisualizeCommand => ({
  databaseRef,
  sql: 'SELECT 1 AS n',
  view: 'table',
  limit: 50,
});

describe('dispatch visualize', () => {
  beforeEach(() => {
    const cleanup = registerExecutor({
      databaseId: 'reset',
      runQuery: async () => ({
        runId: 'r',
        status: 'success' as const,
        rowCount: 0,
      }),
      getRunData: async () => ({ rows: [], columns: [] }),
      cancelQuery: () => {},
    });
    cleanup();
  });

  afterEach(() => {
    const cleanup = registerExecutor({
      databaseId: 'reset',
      runQuery: async () => ({
        runId: 'r',
        status: 'success' as const,
        rowCount: 0,
      }),
      getRunData: async () => ({ rows: [], columns: [] }),
      cancelQuery: () => {},
    });
    cleanup();
  });

  test('resolves a known ref to its id and selects it', async () => {
    const { deps, selected } = makeDeps(['db1']);
    registerStubExecutor('db1');
    const result = (await dispatch(
      'visualize',
      visualizeCmd('db1'),
      deps,
      () => null,
    )) as VisualizeResult;
    expect(selected).toEqual(['db1']);
    expect(result.runId).toBe('run-1');
    expect(result.rowCount).toBe(1);
  });

  test('reports the true total row count, not the capped number read', async () => {
    // The run produced 10,000 rows but only 50 were read back (the cap). The
    // result must report the total (10,000), not the capped read (50), so the
    // agent knows the output was truncated.
    const { deps } = makeDeps(['db1']);
    registerStubExecutor('db1', 10_000, 50);
    const result = (await dispatch(
      'visualize',
      visualizeCmd('db1'),
      deps,
      () => null,
    )) as VisualizeResult;
    expect(result.rowCount).toBe(10_000);
    expect(result.rows.length).toBe(50);
  });

  test('reports a chart render failure as chartError but still returns rows', async () => {
    // ECharts isn't loaded in the test environment, so rendering fails. The
    // dispatcher must surface that as chartError without failing the call or
    // dropping the run data.
    const { deps } = makeDeps(['db1']);
    registerStubExecutor('db1');
    const result = (await dispatch(
      'visualize',
      { ...visualizeCmd('db1'), view: 'chart' },
      deps,
      () => null,
    )) as VisualizeResult;
    expect(result.runId).toBe('run-1');
    expect(result.rowCount).toBe(1);
    expect(result.image).toBeUndefined();
    expect(result.chartError).toBeTruthy();
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
      () => null,
    )) as VisualizeResult;
    expect(selected).toEqual(['db-unlisted']);
    expect(result.runId).toBe('run-1');
  });
});
