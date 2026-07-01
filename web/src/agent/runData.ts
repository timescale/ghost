import type { ChartColumn, ChartData } from '../components/chart/types';

// Minimal shape of the in-process results-cache client exposed by the widget's
// ResultsCacheContext. The full type isn't re-exported by the package, so we
// model just the `rpc` surface we use. `getRunInfo` yields the run's column
// `fields` (including their Postgres types); `readCache` yields the rows.
export interface ResultsCacheClient {
  rpc(payload: {
    type: 'getRunInfo' | 'readCache' | 'deleteRun';
    data: unknown;
  }): Promise<{ data: unknown; error?: string }>;
}

interface RunField {
  name: string;
  type?: string;
}

interface RunInfo {
  fields?: RunField[];
}

interface CachedResult {
  rows?: Record<string, unknown>[];
}

// fetchRunData reads the cached results for a completed run (by runId) out of
// the widget's in-process results cache. Returns the rows + columns. Shared by
// the chart hook (for live rendering) and the agent layer (for screenshots and
// returning rows to the MCP tool). `limit` caps the number of rows read.
export async function fetchRunData(
  client: ResultsCacheClient,
  runId: string,
  limit: number,
): Promise<ChartData> {
  const info = await client.rpc({ type: 'getRunInfo', data: { runId } });
  if (info.error) throw new Error(info.error);
  const fields = (info.data as RunInfo | null)?.fields ?? [];

  const cache = await client.rpc({
    type: 'readCache',
    data: { runId, fields, limit, offset: 0 },
  });
  if (cache.error) throw new Error(cache.error);

  const rows = (cache.data as CachedResult[] | undefined)?.[0]?.rows ?? [];
  // Prefer the run's declared fields for column order/types; fall back to the
  // keys of the first row when no fields are available.
  const columns: ChartColumn[] = fields.length
    ? fields.map((f) => ({ name: f.name, type: f.type }))
    : Object.keys(rows[0] ?? {}).map((name) => ({ name }));

  return { rows, columns };
}

// deleteRun evicts a completed run's results from the widget's in-process
// results cache (dropping its DuckDB table). Used to enforce the query-history
// retention limit by removing the oldest run once a new one pushes past it.
export async function deleteRun(
  client: ResultsCacheClient,
  runId: string,
): Promise<void> {
  const res = await client.rpc({ type: 'deleteRun', data: { runId } });
  if (res.error) throw new Error(res.error);
}

// evictRuns best-effort evicts each run's cached results from the widget cache.
// A failed delete only leaks a cached run (reclaimed on page reload), so errors
// are swallowed. No-op when the client isn't ready.
export function evictRuns(
  client: ResultsCacheClient | null,
  runIds: Iterable<string>,
): void {
  if (!client) return;
  for (const runId of runIds) {
    void deleteRun(client, runId).catch(() => {
      // Best-effort: a leaked cached run is reclaimed on reload.
    });
  }
}

// rowsToMatrix converts row objects (keyed by column name) into a positional
// matrix aligned to the given columns — the [][]any shape the agent returns to
// the MCP tool.
export function rowsToMatrix(
  rows: Record<string, unknown>[],
  columns: ChartColumn[],
): unknown[][] {
  return rows.map((row) => columns.map((col) => row[col.name] ?? null));
}
