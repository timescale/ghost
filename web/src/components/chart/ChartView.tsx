import { useEffect, useRef, useState } from 'react';

import { buildChartOption } from './buildChartOption';
import { type EChartsInstance, getECharts } from './echarts';
import type { ChartData } from './types';

interface Props {
  data: ChartData | null;
  loading: boolean;
  dataError: string | null;
  config: string;
}

// ChartView renders an Apache ECharts chart from the query results and the
// user-authored config. It owns the chart instance lifecycle (init/resize/
// dispose) and re-applies the option whenever the data or config changes,
// surfacing any config-evaluation error as an overlay.
export function ChartView({ data, loading, dataError, config }: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const chartRef = useRef<EChartsInstance | null>(null);
  const [renderError, setRenderError] = useState<string | null>(null);
  const echarts = getECharts();

  // Initialize the chart instance once, and keep it sized to its container.
  useEffect(() => {
    const el = containerRef.current;
    if (!el || !echarts) return;
    const chart = echarts.init(el);
    chartRef.current = chart;
    const observer = new ResizeObserver(() => chart.resize());
    observer.observe(el);
    return () => {
      observer.disconnect();
      chart.dispose();
      chartRef.current = null;
    };
  }, [echarts]);

  // Re-apply the option whenever the data or config changes.
  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;
    if (!data) {
      chart.clear();
      setRenderError(null);
      return;
    }
    try {
      chart.setOption(buildChartOption(config, data), { notMerge: true });
      setRenderError(null);
    } catch (err) {
      chart.clear();
      setRenderError(err instanceof Error ? err.message : String(err));
    }
  }, [data, config]);

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
