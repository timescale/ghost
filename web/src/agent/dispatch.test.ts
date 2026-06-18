import { afterEach, beforeEach, describe, expect, test } from 'bun:test';

import type { ResultView } from '../components/chart/types';
import { type DispatchDeps, dispatch } from './dispatch';
import { type Executor, registerExecutor } from './executor';
import type { VisualizeCommand, VisualizeResult } from './types';

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

function registerStubExecutor(databaseId: string): void {
  const executor: Executor = {
    databaseId,
    runQuery: async () => ({ runId: 'run-1', status: 'success' as const }),
    getRunData: async () => ({
      rows: [{ n: 1 }],
      columns: [{ name: 'n', type: 'INT8' }],
    }),
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
      runQuery: async () => ({ runId: 'r', status: 'success' as const }),
      getRunData: async () => ({ rows: [], columns: [] }),
    });
    cleanup();
  });

  afterEach(() => {
    const cleanup = registerExecutor({
      databaseId: 'reset',
      runQuery: async () => ({ runId: 'r', status: 'success' as const }),
      getRunData: async () => ({ rows: [], columns: [] }),
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
