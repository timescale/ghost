import type { ChartData } from '../components/chart/types';

// QueryOutcome is the result of running a query in the browser.
export interface QueryOutcome {
  runId: string;
  status: 'success' | 'failed';
  // Total number of rows the query produced, as reported by the widget on
  // completion. This is the true total, independent of any row cap applied when
  // reading results back for the agent.
  rowCount: number;
  // Postgres command-tag count for the run (rows touched by a DML command, or
  // rows returned by a SELECT), as reported by the widget. Matches Go's
  // common.ExecuteQuery RowsAffected so the structured tool output is accurate
  // whether or not the query was visualized. Zero for a failed/canceled run.
  rowsAffected: number;
  // Postgres command tag for the run (e.g. "SELECT", "INSERT", "CREATE"), as
  // reported by the widget. Mirrors common.ResultSet.CommandTag from the
  // server-side path so visualized results carry the same command_tag. Empty
  // string when the widget reported no command (e.g. a failed parse).
  commandTag: string;
  error?: string;
}

// Executor is the per-database capability surface the agent dispatcher drives.
// It is implemented inside QueryPanel (which owns the widget apiRef and the
// results-cache client) and registered here keyed by database ID, so the
// app-level command dispatcher can reach it across database-switch remounts.
export interface Executor {
  databaseId: string;
  // Run the given SQL, resolving once the run completes (success or failure).
  runQuery(sql: string): Promise<QueryOutcome>;
  // Read the cached results for a completed run, capped at `limit` rows.
  getRunData(runId: string, limit: number): Promise<ChartData>;
  // Abort the query currently running (if any). Used when the agent bridge
  // signals that the MCP caller canceled, timed out, or another tab took over.
  cancelQuery(): void;
}

let current: Executor | null = null;
const waiters = new Set<(executor: Executor) => void>();

// registerExecutor installs the executor for the currently-mounted database and
// notifies anyone awaiting it. Returns an unregister function for cleanup.
// Waiters are notified from a snapshot (so a waiter re-registering itself for a
// different database isn't lost), and each satisfied waiter removes itself.
export function registerExecutor(executor: Executor): () => void {
  current = executor;
  for (const resolve of [...waiters]) resolve(executor);
  return () => {
    if (current === executor) current = null;
  };
}

// getExecutor returns the executor for the currently-mounted database, or null.
export function getExecutor(): Executor | null {
  return current;
}

// awaitExecutor resolves with the executor for the given database ID. If the
// matching executor is already mounted it resolves immediately; otherwise it
// waits for one to register (e.g. after the agent switches the selected
// database and QueryPanel remounts), rejecting if `timeoutMs` elapses first.
export function awaitExecutor(
  databaseId: string,
  timeoutMs: number,
): Promise<Executor> {
  if (current && current.databaseId === databaseId) {
    return Promise.resolve(current);
  }
  return new Promise<Executor>((resolve, reject) => {
    const timer = setTimeout(() => {
      waiters.delete(onRegister);
      reject(new Error('timed out waiting for the database panel to load'));
    }, timeoutMs);
    const onRegister = (executor: Executor) => {
      // Keep waiting until the database we want mounts.
      if (executor.databaseId !== databaseId) return;
      clearTimeout(timer);
      waiters.delete(onRegister);
      resolve(executor);
    };
    waiters.add(onRegister);
  });
}
