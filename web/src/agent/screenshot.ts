import { buildChartOption } from '../components/chart/buildChartOption';
import { getECharts } from '../components/chart/echarts';
import type { ChartData } from '../components/chart/types';

// Fixed pixel size for agent-facing chart screenshots. Rendered at 2x for a
// crisp image the agent can inspect.
const CHART_WIDTH = 1200;
const CHART_HEIGHT = 800;
const PIXEL_RATIO = 2;

// renderChartImage evaluates the chart config against the data and renders it
// to a PNG data URL, off-screen. It creates a detached, fixed-size container,
// initializes a throwaway ECharts instance, applies the option, captures the
// image, and tears everything down. Throws if the charting library isn't
// loaded or the config/option is invalid (so the caller can report the error).
export async function renderChartImage(
  config: string,
  data: ChartData,
): Promise<string> {
  const echarts = getECharts();
  if (!echarts) {
    throw new Error('charting library failed to load');
  }

  // Build the option first so a config error surfaces before we touch the DOM.
  // Force animation off for the capture: ECharts animates the initial render,
  // and getDataURL grabs whatever is on the canvas at that instant — so an
  // animated chart is usually captured mid-transition (a partial graph).
  // Disabling animation makes the first painted frame the final one. This only
  // affects the off-screen screenshot, never the live on-screen chart.
  const option = { ...buildChartOption(config, data), animation: false };

  // getDataURL's backgroundColor *overrides* the option's backgroundColor for
  // the exported image. ECharts paints a transparent background by default, so
  // we fall back to white for configs that don't set one — but a config that
  // does set a backgroundColor (e.g. a dark theme, or a gradient/pattern
  // object) must be honored, otherwise the on-screen chart and the
  // agent-facing screenshot disagree. Reuse the config's backgroundColor when
  // it's set (a non-empty string or an object); else default to white.
  const optionBackground = option.backgroundColor;
  const hasBackground =
    typeof optionBackground === 'string'
      ? optionBackground !== ''
      : optionBackground != null && typeof optionBackground === 'object';
  const backgroundColor = hasBackground ? optionBackground : '#ffffff';

  const container = document.createElement('div');
  container.style.position = 'absolute';
  container.style.left = '-10000px';
  container.style.top = '0';
  container.style.width = `${CHART_WIDTH}px`;
  container.style.height = `${CHART_HEIGHT}px`;
  document.body.appendChild(container);

  // Append to the DOM before init so that echarts.init() throwing (or any other
  // error below) can't leak the detached container — container.remove() always
  // runs in the outer finally.
  try {
    const chart = echarts.init(container, undefined, {
      width: CHART_WIDTH,
      height: CHART_HEIGHT,
    });
    try {
      chart.setOption(option, { notMerge: true });
      // Wait for the render to flush before capturing. ECharts renders
      // synchronously on setOption by default, but a rAF tick ensures any
      // deferred layout (e.g. animations disabled) has settled.
      await new Promise((resolve) =>
        requestAnimationFrame(() => resolve(null)),
      );
      return chart.getDataURL({
        type: 'png',
        pixelRatio: PIXEL_RATIO,
        backgroundColor,
      });
    } finally {
      chart.dispose();
    }
  } finally {
    container.remove();
  }
}
