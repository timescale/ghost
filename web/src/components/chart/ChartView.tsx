import { useEffect, useMemo, useRef, useState } from 'react';

import { debounce } from '../../util/debounce';
import { buildChartOption } from './buildChartOption';
import {
  type EChartsGlobal,
  type EChartsInstance,
  getECharts,
} from './echarts';
import type { ChartData } from './types';

interface Props {
  data: ChartData | null;
  loading: boolean;
  dataError: string | null;
  config: string;
  // Called after the config renders successfully against non-empty data, with
  // the exact config that rendered. Used to record chart config history. Not
  // called for empty data or render errors.
  onRenderSuccess?: (config: string) => void;
}

// applyChartOption evaluates the config against the data and applies the
// resulting option to the chart instance. It takes everything it needs as args,
// so there are no closures over component state to go stale. chartRef is a
// plain mutable box ({ current }); on error we recreate the instance and write
// it back. The component debounces this per-instance (see useMemo below) rather
// than at module scope, so multiple ChartViews (e.g. the main chart and the
// history modal's preview) don't share a single timer and clobber each other.
//
// Because a chart config may be async (buildChartOption resolves a Promise),
// applies can complete out of order. Each call claims a sequence number from
// seqRef up front; after awaiting the config it bails if a newer apply has
// claimed the counter (or the view unmounted), so a slow async config can
// never clobber a newer render or clear.
async function applyChartOption(
  echarts: EChartsGlobal,
  chartRef: { current: EChartsInstance | null },
  seqRef: { current: number },
  containerEl: HTMLElement | null,
  data: ChartData | null,
  config: string,
  setRenderError: (message: string | null) => void,
  onRenderSuccess: ((config: string) => void) | undefined,
): Promise<void> {
  const seq = ++seqRef.current;
  if (!chartRef.current) return;
  if (!data) {
    chartRef.current.clear();
    setRenderError(null);
    return;
  }
  try {
    const option = await buildChartOption(config, data, echarts);
    // The data/config changed again, or the view unmounted, while the config
    // was being built — drop the stale option.
    if (seq !== seqRef.current || !chartRef.current) return;
    chartRef.current.setOption(option, { notMerge: true });
    setRenderError(null);
    // Only a config that renders against real rows is worth remembering. Pass
    // the config that actually rendered, so a later debounced record can't
    // capture a different (e.g. invalid, not-yet-rendered) config from the
    // store if the user edited it in the meantime.
    if (data.rows.length > 0) onRenderSuccess?.(config);
  } catch (err) {
    if (seq !== seqRef.current || !chartRef.current) return;
    // A failed setOption can throw mid-render and leave ECharts in an
    // inconsistent internal state that a later clear()/setOption won't recover
    // from (the next apply silently renders nothing or stale content). Dispose
    // and recreate the instance so the next valid config renders cleanly.
    chartRef.current.dispose();
    chartRef.current = containerEl ? echarts.init(containerEl) : null;
    setRenderError(err instanceof Error ? err.message : String(err));
  }
}

// ChartView renders an Apache ECharts chart from the query results and the
// user-authored config. It owns the chart instance lifecycle (init/resize/
// dispose) and re-applies the option whenever the data or config changes,
// surfacing any config-evaluation error as an overlay.
export function ChartView({
  data,
  loading,
  dataError,
  config,
  onRenderSuccess,
}: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const chartRef = useRef<EChartsInstance | null>(null);
  // Monotonic apply counter shared with applyChartOption's stale-result guard.
  const applySeqRef = useRef(0);
  const [renderError, setRenderError] = useState<string | null>(null);
  const echarts = getECharts();

  // Per-instance debounced apply, so concurrent ChartViews don't share a timer.
  const debouncedApply = useMemo(() => debounce(applyChartOption, 200), []);

  // Initialize the chart instance once, and keep it sized to its container.
  // The ResizeObserver reads chartRef.current (rather than capturing the
  // instance) so it resizes whichever instance is current, even after the
  // error-recovery reinit below replaces it.
  useEffect(() => {
    const el = containerRef.current;
    if (!el || !echarts) return;
    chartRef.current = echarts.init(el);
    const observer = new ResizeObserver(() => chartRef.current?.resize());
    observer.observe(el);
    return () => {
      observer.disconnect();
      chartRef.current?.dispose();
      chartRef.current = null;
    };
  }, [echarts]);

  // Re-apply (debounced) whenever the data or config changes; cancel any
  // pending apply on unmount.
  useEffect(() => {
    if (!echarts) return;
    debouncedApply(
      echarts,
      chartRef,
      applySeqRef,
      containerRef.current,
      data,
      config,
      setRenderError,
      onRenderSuccess,
    );
    return debouncedApply.cancel;
  }, [echarts, data, config, onRenderSuccess, debouncedApply]);

  if (!echarts) {
    return (
      <div className="flex flex-auto items-center justify-center p-4 text-center text-sm text-slate-500">
        Charting library failed to load. Check your network connection and
        reload.
      </div>
    );
  }

  const overlay = renderError
    ? { tone: 'error' as const, text: renderError }
    : dataError
      ? { tone: 'error' as const, text: dataError }
      : loading
        ? { tone: 'muted' as const, text: 'Loading results…' }
        : !data
          ? {
              tone: 'muted' as const,
              text: 'Run a query to chart its results.',
            }
          : data.rows.length === 0
            ? { tone: 'muted' as const, text: 'Query returned no rows.' }
            : null;

  return (
    <div className="relative flex-auto overflow-hidden">
      <div ref={containerRef} className="h-full w-full" />
      {overlay ? (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-white/70 p-4">
          <pre
            className={`max-w-full overflow-auto whitespace-pre-wrap text-center text-sm ${
              overlay.tone === 'error' ? 'text-red-600' : 'text-slate-500'
            }`}
          >
            {overlay.text}
          </pre>
        </div>
      ) : null}
    </div>
  );
}
