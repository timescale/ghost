import { afterEach, beforeEach, describe, expect, mock, test } from 'bun:test';

import type { ChartData } from '../components/chart/types';

// Capture what option/getDataURL args the chart instance receives so we can
// assert how the screenshot background is resolved.
interface Capture {
  option: Record<string, unknown> | null;
  getDataURLArg: Record<string, unknown> | null;
}

const capture: Capture = { option: null, getDataURLArg: null };

function installECharts(): void {
  (globalThis as unknown as { window: Record<string, unknown> }).window = {
    echarts: {
      init: () => ({
        setOption: (option: Record<string, unknown>) => {
          capture.option = option;
        },
        getDataURL: (arg: Record<string, unknown>) => {
          capture.getDataURLArg = arg;
          return 'data:image/png;base64,STUB';
        },
        dispose: () => {},
      }),
    },
  };
}

beforeEach(() => {
  capture.option = null;
  capture.getDataURLArg = null;
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

  test('falls back to white when the config sets no backgroundColor', async () => {
    const { renderChartImage } = await import('./screenshot');
    const config = `function chart(data) {
      return { series: [] };
    }`;
    await renderChartImage(config, data);
    expect(capture.getDataURLArg?.backgroundColor).toBe('#ffffff');
  });
});
