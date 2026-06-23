import type { EChartsOption } from 'echarts';

import { buildChartOption } from '../components/chart/buildChartOption';
import { type EChartsInstance, getECharts } from '../components/chart/echarts';
import type { ChartData } from '../components/chart/types';

// The chart option we render, with animation forced off for the capture.
type CaptureOption = EChartsOption & { animation: false };
// ECharts' getDataURL accepts the same color shape as a config's
// backgroundColor (a color string, or a gradient/pattern object).
type BackgroundColor = EChartsOption['backgroundColor'];

// Fixed logical size for agent-facing chart screenshots, plus the export scale
// factor. The exported PNG is CHART_WIDTH*PIXEL_RATIO x CHART_HEIGHT*PIXEL_RATIO
// (1920x1280). pixelRatio is a layout-zoom factor, not a sharpness knob: ECharts
// lays the chart out in these logical pixels (font sizes, axis spacing, label
// thinning) and scales the result on export. The long edge must stay <= 2000 px:
// Anthropic's vision API drops its per-image dimension cap from 8000 to 2000 px
// once a request carries more than 20 images ("many-image requests"), and a
// chart-heavy agent session easily exceeds that, so a larger image is rejected
// with an invalid_request_error mid-conversation. 1920 leaves margin under 2000
// while staying close to the model's native resolution (no wasted detail).
const CHART_WIDTH = 1200;
const CHART_HEIGHT = 800;
const PIXEL_RATIO = 1.6;

// Maximum time to wait for ECharts to finish rendering before capturing anyway.
// The 'finished' event normally fires within a frame or two; this is only a
// safety net so a chart that somehow never settles can't hang the capture (the
// agent bridge enforces its own request timeout on top of this).
const RENDER_TIMEOUT_MS = 10_000;

// renderToDataURL applies the option and resolves with the captured PNG only
// once ECharts has *finished* rendering. ECharts renders large series
// progressively, across multiple animation frames (driven by the per-series
// `progressive`/`progressiveThreshold` options, default 3000 elements), so a
// single requestAnimationFrame tick isn't enough — getDataURL would grab a
// partial graph. The 'finished' event fires once the chart goes idle (all
// progressive chunks painted and any animation settled), which is the correct
// signal that the fresh result set has been fully rendered.
//
// The listener is attached *before* setOption so the event can't be missed:
// ECharts emits 'finished' asynchronously on a later frame, never synchronously
// within setOption. A timeout fallback captures whatever is rendered rather
// than hanging, should 'finished' never arrive.
function renderToDataURL(
  chart: EChartsInstance,
  option: CaptureOption,
  backgroundColor: BackgroundColor,
): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    let settled = false;
    const onFinished = () => finish();
    const timer = setTimeout(() => finish(), RENDER_TIMEOUT_MS);

    // Tear down the listener and timeout on every settle path, so neither
    // lingers after the promise resolves/rejects. Without this, a fast resolve
    // would leave the 10s timer armed, and an early reject (e.g. setOption
    // throwing below) would leave it to later fire getDataURL on a chart the
    // caller has already disposed — an unhandled async exception.
    const cleanup = () => {
      chart.off('finished', onFinished);
      clearTimeout(timer);
    };
    // finish runs once (whichever of 'finished' or the timeout wins). A throw
    // from getDataURL (e.g. a tainted canvas) rejects rather than escaping.
    function finish() {
      if (settled) return;
      settled = true;
      cleanup();
      try {
        resolve(
          chart.getDataURL({
            type: 'png',
            pixelRatio: PIXEL_RATIO,
            backgroundColor,
          }),
        );
      } catch (err) {
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    }

    chart.on('finished', onFinished);
    try {
      chart.setOption(option, { notMerge: true });
    } catch (err) {
      // setOption can throw on a malformed (but object-shaped) option that
      // buildChartOption couldn't catch. Reject cleanly instead of leaving the
      // listener/timer dangling.
      if (!settled) {
        settled = true;
        cleanup();
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    }
  });
}

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
  // Disabling animation removes the animation phase; we still wait for the
  // 'finished' event (see renderToDataURL) to cover progressive rendering. This
  // only affects the off-screen screenshot, never the live on-screen chart.
  const option: CaptureOption = {
    ...buildChartOption(config, data),
    animation: false,
  };

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
  const backgroundColor: BackgroundColor = hasBackground
    ? optionBackground
    : '#ffffff';

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
      // Wait for ECharts to finish rendering (including progressive rendering
      // of large result sets) before capturing, so the screenshot always
      // reflects the fully-rendered fresh data rather than a partial frame.
      return await renderToDataURL(chart, option, backgroundColor);
    } finally {
      chart.dispose();
    }
  } finally {
    container.remove();
  }
}
