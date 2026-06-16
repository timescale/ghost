// Accessor for the Apache ECharts global loaded via the CDN <script> tag in
// index.html. echarts is a devDependency (types only — it is not bundled; the
// runtime comes from the CDN), so we use its official types here.
import type * as echarts from 'echarts';

// The global namespace and the chart instance type, sourced from echarts'
// official type definitions.
export type EChartsGlobal = typeof echarts;
export type EChartsInstance = echarts.ECharts;

declare global {
  interface Window {
    echarts?: EChartsGlobal;
  }
}

// getECharts returns the global ECharts namespace, or null if the CDN script
// hasn't loaded (e.g. offline). Callers should surface a friendly message.
export function getECharts(): EChartsGlobal | null {
  return window.echarts ?? null;
}
