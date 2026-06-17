import { ResultsCacheContext } from '@timescale/popsql-query-widget-cdn';
import { useContext, useEffect, useState } from 'react';

import type { ChartColumn, ChartData } from './types';

// Cap the number of rows pulled into a chart. ECharts degrades badly past tens
// of thousands of points, and a chart over a million rows isn't meaningful
// anyway. Users who need more should aggregate in SQL first.
const MAX_CHART_ROWS = 50_000;

// Minimal shape of the in-process results-cache client exposed by the widget's
// ResultsCacheContext. The full type isn't re-exported by the package, so we
// model just the `rpc` surface we use. `getRunInfo` yields the run's column
// `fields` (including their Postgres types); `readCache` yields the rows.
interface ResultsCacheClient {
  rpc(payload: {
    type: 'getRunInfo' | 'readCache';
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

interface State {
  data: ChartData | null;
  loading: boolean;
  error: string | null;
}

const IDLE: State = { data: null, loading: false, error: null };

// useChartData reads the cached results for a completed run (by runId) out of
// the widget's in-process results cache, so charting reuses the rows already
// fetched by the query — no re-execution. Returns the rows + columns, a loading
// flag, and any error.
export function useChartData(runId: string | null): State {
  const { client } = useContext(ResultsCacheContext) as {
    client: ResultsCacheClient | null;
  };
  const [state, setState] = useState<State>(IDLE);

  useEffect(() => {
    if (!runId || !client) {
      setState(IDLE);
      return;
    }
    let cancelled = false;
    setState({ data: null, loading: true, error: null });

    (async () => {
      const info = await client.rpc({ type: 'getRunInfo', data: { runId } });
      if (info.error) throw new Error(info.error);
      const fields = (info.data as RunInfo | null)?.fields ?? [];

      const cache = await client.rpc({
        type: 'readCache',
        data: { runId, fields, limit: MAX_CHART_ROWS, offset: 0 },
      });
      if (cache.error) throw new Error(cache.error);
      if (cancelled) return;

      const rows = (cache.data as CachedResult[] | undefined)?.[0]?.rows ?? [];
      // Prefer the run's declared fields for column order/types; fall back to
      // the keys of the first row when no fields are available.
      const columns: ChartColumn[] = fields.length
        ? fields.map((f) => ({ name: f.name, type: f.type }))
        : Object.keys(rows[0] ?? {}).map((name) => ({ name }));

      setState({ data: { rows, columns }, loading: false, error: null });
    })().catch((err: unknown) => {
      if (cancelled) return;
      setState({
        data: null,
        loading: false,
        error: err instanceof Error ? err.message : String(err),
      });
    });

    return () => {
      cancelled = true;
    };
  }, [runId, client]);

  return state;
}
