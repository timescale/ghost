import type { EChartsOption } from 'echarts';

import type { EChartsGlobal } from './echarts';
import type { ChartData } from './types';

// buildChartOption evaluates the user-authored chart config — which defines a
// `chart(data, echarts)` function — against the query results and resolves
// with the resulting ECharts option object. The function receives the ECharts
// namespace as its second argument (the same object as the global `echarts`),
// e.g. for echarts.registerMap(...), and may return the option directly or a
// Promise of it — an async config can fetch resources (like map GeoJSON) and
// register them before returning the option.
//
// We run the code directly via `new Function` rather than in a hardened sandbox
// (iframe/worker): the person authoring this config is the operator of their
// own local `ghost serve` instance — they can already run arbitrary SQL against
// their database — so the config introduces no new trust boundary. Errors are
// thrown to the caller, which surfaces them in the chart pane.
//
// Note: with the MCP agent bridge, a `chart_config` can also originate from an
// AI agent (via `ghost_visualize`) rather than being hand-authored by
// the operator. This is a deliberate, accepted trade-off: the whole stack runs
// on localhost, driven by an MCP server the operator chose to connect to a
// model they trust, and that model can already issue arbitrary SQL. So an
// agent-synthesized config executing here grants no capability beyond what the
// operator has already delegated, and the blast radius is the operator's own
// local machine. We keep direct `new Function` evaluation for simplicity rather
// than sandboxing it.
//
// The config is plain JavaScript (the editor's TypeScript checking is driven by
// JSDoc, so no transpilation is needed); we append a `return` to hand back the
// declared `chart` function.
export async function buildChartOption(
  code: string,
  data: ChartData,
  echarts: EChartsGlobal,
): Promise<EChartsOption> {
  const factory = new Function(
    `${code}\nreturn typeof chart === 'function' ? chart : null;`,
  );
  const fn = factory() as
    | ((
        d: ChartData,
        e: EChartsGlobal,
      ) => EChartsOption | Promise<EChartsOption>)
    | null;
  if (typeof fn !== 'function') {
    throw new Error("chart config must define a function named 'chart'");
  }
  // Await tolerates both shapes: a sync config's plain option passes through
  // unchanged, an async (or Promise-returning) config resolves first.
  const option = await fn(data, echarts);
  if (option == null || typeof option !== 'object') {
    throw new Error('chart config must return an ECharts option object');
  }
  return option;
}
