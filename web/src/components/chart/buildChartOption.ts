import type { EChartsOption } from 'echarts';

import type { ChartData } from './types';

// buildChartOption evaluates the user-authored chart config — which defines a
// `chart(data)` function — against the query results and returns the resulting
// ECharts option object.
//
// We run the code directly via `new Function` rather than in a hardened sandbox
// (iframe/worker): the person authoring this config is the operator of their
// own local `ghost serve` instance — they can already run arbitrary SQL against
// their database — so the config introduces no new trust boundary. Errors are
// thrown to the caller, which surfaces them in the chart pane.
//
// The config is plain JavaScript (the editor's TypeScript checking is driven by
// JSDoc, so no transpilation is needed); we append a `return` to hand back the
// declared `chart` function.
export function buildChartOption(code: string, data: ChartData): EChartsOption {
  const factory = new Function(
    `${code}\nreturn typeof chart === 'function' ? chart : null;`,
  );
  const fn = factory() as ((d: ChartData) => EChartsOption) | null;
  if (typeof fn !== 'function') {
    throw new Error("chart config must define a function named 'chart'");
  }
  const option = fn(data);
  if (option == null || typeof option !== 'object') {
    throw new Error('chart config must return an ECharts option object');
  }
  return option;
}
