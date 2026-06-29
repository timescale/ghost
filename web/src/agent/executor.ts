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
  error?: string;
}

// Executor is the per-database capability surface the agent dispatcher drives.
// It is implemented inside QueryPanel (which owns the widget apiRef and the
// results-cache client) and registered here keyed by database ID, so the
// app-level command dispatcher can reach it across database-switch remounts.
export interface Executor {
  databaseId: string;
  // Run the given SQL, resolving once the run completes (success or failure).
  // Aborting `signal` cancels this run's in-flight query — and only this run's,
  // never an unrelated query. Once a run has started, cancellation settles the
  // promise by *resolving* it with `{status: 'failed', error: 'the query was
  // canceled'}` (the canceled completion), not by rejecting; the promise only
  // rejects in pre-start paths (already aborted, or the panel torn down / a
  // query already running before this one starts). Used when the agent bridge
  // signals the MCP request was canceled, timed out, or superseded.
  runQuery(sql: string, signal: AbortSignal): Promise<QueryOutcome>;
  // Read the cached results for a completed run, capped at `limit` rows.
  getRunData(runId: string, limit: number): Promise<ChartData>;
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
// If `signal` is provided and aborts while waiting, it rejects promptly (and
// drops its waiter) so an abandoned command doesn't keep waiting for a panel
// that may never mount — mirroring runQuery's already-aborted bail.
export function awaitExecutor(
  databaseId: string,
  timeoutMs: number,
  signal?: AbortSignal,
): Promise<Executor> {
  if (current && current.databaseId === databaseId) {
    return Promise.resolve(current);
  }
  if (signal?.aborted) {
    return Promise.reject(new Error('the command was canceled'));
  }
  return new Promise<Executor>((resolve, reject) => {
    const cleanup = () => {
      clearTimeout(timer);
      waiters.delete(onRegister);
      signal?.removeEventListener('abort', onAbort);
    };
    const timer = setTimeout(() => {
      cleanup();
      reject(new Error('timed out waiting for the database panel to load'));
    }, timeoutMs);
    const onAbort = () => {
      cleanup();
      reject(new Error('the command was canceled'));
    };
    const onRegister = (executor: Executor) => {
      // Keep waiting until the database we want mounts.
      if (executor.databaseId !== databaseId) return;
      cleanup();
      resolve(executor);
    };
    waiters.add(onRegister);
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}
