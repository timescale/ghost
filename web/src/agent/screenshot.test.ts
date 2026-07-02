import {
  afterEach,
  beforeEach,
  describe,
  expect,
  jest,
  mock,
  test,
} from 'bun:test';

import type { ChartData } from '../components/chart/types';

// Capture what option/getDataURL args the chart instance receives so we can
// assert how the screenshot background is resolved.
interface Capture {
  option: Record<string, unknown> | null;
  getDataURLArg: Record<string, unknown> | null;
  // Order of lifecycle events observed on the stub chart instance, used to
  // assert that the image is captured only after rendering has 'finished'.
  events: string[];
}

const capture: Capture = { option: null, getDataURLArg: null, events: [] };

// installECharts installs a stub ECharts global. The stub models the
// 'finished' event: setOption schedules it on a microtask (as real ECharts
// fires it asynchronously on a later frame, never synchronously within
// setOption), and renderToDataURL must wait for it before calling getDataURL.
// When fireFinished is false, the event never fires, exercising the timeout
// fallback.
function installECharts(fireFinished = true, setOptionThrows = false): void {
  (globalThis as unknown as { window: Record<string, unknown> }).window = {
    echarts: {
      init: () => {
        const listeners: Record<string, () => void> = {};
        return {
          on: (event: string, handler: () => void) => {
            listeners[event] = handler;
          },
          off: (event: string) => {
            delete listeners[event];
          },
          setOption: (option: Record<string, unknown>) => {
            capture.events.push('setOption');
            if (setOptionThrows) {
              throw new Error('bad option');
            }
            capture.option = option;
            if (fireFinished) {
              queueMicrotask(() => {
                capture.events.push('finished');
                listeners.finished?.();
              });
            }
          },
          getDataURL: (arg: Record<string, unknown>) => {
            capture.getDataURLArg = arg;
            capture.events.push('getDataURL');
            return 'data:image/png;base64,STUB';
          },
          dispose: () => {
            capture.events.push('dispose');
          },
        };
      },
    },
  };
}

beforeEach(() => {
  capture.option = null;
  capture.getDataURLArg = null;
  capture.events = [];
  // Minimal DOM stubs: renderChartImage creates a detached container and
  // appends/removes it from document.body.
  (globalThis as unknown as { document: unknown }).document = {
    createElement: () => ({ style: {}, remove: () => {} }),
    body: { appendChild: () => {} },
  };
  (
    globalThis as unknown as { requestAnimationFrame: unknown }
  ).requestAnimationFrame = (cb: (t: number) => void) => {
    cb(0);
    return 0;
  };
  installECharts();
});

afterEach(() => {
  mock.restore();
});

const data: ChartData = { columns: ['x', 'y'], rows: [{ x: 1, y: 2 }] };

describe('renderChartImage', () => {
  test('respects a dark backgroundColor set by the config', async () => {
    const { renderChartImage } = await import('./screenshot');
    const config = `function chart(data) {
      return { backgroundColor: '#1e1e1e', series: [] };
    }`;
    await renderChartImage(config, data);
    // The exported image's background should match the config, not be forced
    // back to white.
    expect(capture.getDataURLArg?.backgroundColor).toBe('#1e1e1e');
  });

  test('respects a gradient backgroundColor object set by the config', async () => {
    const { renderChartImage } = await import('./screenshot');
    const config = `function chart(data) {
      return {
        backgroundColor: {
          type: 'linear',
          colorStops: [{ offset: 0, color: '#000' }],
        },
        series: [],
      };
    }`;
    await renderChartImage(config, data);
    expect(capture.getDataURLArg?.backgroundColor).toEqual({
      type: 'linear',
      colorStops: [{ offset: 0, color: '#000' }],
    });
  });

  test('supports an async config that uses the echarts second argument', async () => {
    // A config may be async (e.g. fetching map GeoJSON) and may use the
    // ECharts namespace passed as its second argument. The capture must await
    // the resolved option — here proven by the resolved backgroundColor — and
    // the passed-in namespace must be the real global (probed via its init).
    const { renderChartImage } = await import('./screenshot');
    const config = `async function chart(data, echarts) {
      if (typeof echarts.init !== 'function') {
        throw new Error('echarts namespace not passed');
      }
      await Promise.resolve();
      return { backgroundColor: '#123456', series: [] };
    }`;
    await renderChartImage(config, data);
    expect(capture.getDataURLArg?.backgroundColor).toBe('#123456');
  });

  test('falls back to white when the config sets no backgroundColor', async () => {
    const { renderChartImage } = await import('./screenshot');
    const config = `function chart(data) {
      return { series: [] };
    }`;
    await renderChartImage(config, data);
    expect(capture.getDataURLArg?.backgroundColor).toBe('#ffffff');
  });

  test('captures only after rendering has finished', async () => {
    // The screenshot must reflect the fully-rendered chart. Real ECharts renders
    // large series progressively across frames and signals completion with the
    // 'finished' event; getDataURL must run only after it, never on setOption.
    const { renderChartImage } = await import('./screenshot');
    const config = `function chart(data) {
      return { series: [] };
    }`;
    await renderChartImage(config, data);
    expect(capture.events).toEqual([
      'setOption',
      'finished',
      'getDataURL',
      'dispose',
    ]);
  });

  test('rejects and tears down cleanly if setOption throws', async () => {
    // A malformed (but object-shaped) option can make ECharts' setOption throw
    // after buildChartOption succeeds. renderToDataURL must reject without
    // leaving its 'finished' listener or 10s timeout dangling — otherwise the
    // timer would later fire getDataURL on a chart the caller already disposed.
    const { renderChartImage } = await import('./screenshot');
    installECharts(true, true);
    jest.useFakeTimers();
    try {
      const config = `function chart(data) {
        return { series: [] };
      }`;
      await expect(renderChartImage(config, data)).rejects.toThrow(
        'bad option',
      );
      // The chart was disposed by renderChartImage's finally...
      expect(capture.events).toEqual(['setOption', 'dispose']);
      // ...and advancing past the render timeout must NOT fire a late capture
      // (no getDataURL on the disposed instance), proving the timer was cleared.
      jest.advanceTimersByTime(10_000);
      expect(capture.events).toEqual(['setOption', 'dispose']);
    } finally {
      jest.useRealTimers();
    }
  });

  test('captures anyway if rendering never finishes (timeout fallback)', async () => {
    // Should 'finished' never fire, the capture must not hang: a timeout
    // fallback still produces an image (the latest rendered frame). Fake timers
    // let us drive the fallback without waiting out the real timeout.
    const { renderChartImage } = await import('./screenshot');
    installECharts(false);
    jest.useFakeTimers();
    try {
      const config = `function chart(data) {
        return { series: [] };
      }`;
      const promise = renderChartImage(config, data);
      // buildChartOption is async, so renderChartImage arms its render timeout
      // only a few microtasks in (setOption and the timer are registered
      // together). Flush microtasks until setOption has run, then advance past
      // the timeout to trigger the fallback capture.
      while (!capture.events.includes('setOption')) {
        await Promise.resolve();
      }
      jest.advanceTimersByTime(10_000);
      const image = await promise;
      expect(image).toBe('data:image/png;base64,STUB');
      expect(capture.events).toEqual(['setOption', 'getDataURL', 'dispose']);
    } finally {
      jest.useRealTimers();
    }
  });
});
