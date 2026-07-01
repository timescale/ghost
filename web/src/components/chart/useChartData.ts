import { useEffect, useState } from 'react';

import { fetchRunData } from '../../agent/runData';
import { useResultsCacheClient } from '../../agent/useResultsCacheClient';
import type { ChartData } from './types';

// Cap the number of rows pulled into a chart. ECharts degrades badly past tens
// of thousands of points, and a chart over a million rows isn't meaningful
// anyway. Users who need more should aggregate in SQL first.
const MAX_CHART_ROWS = 50_000;

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
  const client = useResultsCacheClient();
  const [state, setState] = useState<State>(IDLE);

  useEffect(() => {
    if (!runId || !client) {
      setState(IDLE);
      return;
    }
    let cancelled = false;
    setState({ data: null, loading: true, error: null });

    (async () => {
      const data = await fetchRunData(client, runId, MAX_CHART_ROWS);
      if (cancelled) return;
      setState({ data, loading: false, error: null });
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
