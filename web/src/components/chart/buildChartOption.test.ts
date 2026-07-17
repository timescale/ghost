import { describe, expect, test } from 'bun:test';

import { buildChartOption } from './buildChartOption';
import { DEFAULT_CHART_CONFIG } from './defaultConfig';
import type { EChartsGlobal } from './echarts';
import type { ChartData } from './types';

const data: ChartData = {
  columns: [{ name: 'x' }, { name: 'y' }],
  rows: [{ x: 1, y: 2 }],
};

// A minimal stand-in for the ECharts namespace; buildChartOption just threads
// it through to the config's second parameter.
const echartsStub = {
  registerMap: () => {},
} as unknown as EChartsGlobal;

describe('buildChartOption', () => {
  test('resolves the option from a sync config', async () => {
    const config = `function chart(data) {
      return { title: { text: 'rows: ' + data.rows.length } };
    }`;
    const option = await buildChartOption(config, data, echartsStub);
    expect(option).toEqual({ title: { text: 'rows: 1' } });
  });

  test('passes the echarts namespace as the second argument', async () => {
    const config = `function chart(data, echarts) {
      return { title: { text: typeof echarts.registerMap } };
    }`;
    const option = await buildChartOption(config, data, echartsStub);
    expect(option).toEqual({ title: { text: 'function' } });
  });

  test('resolves the option from an async config', async () => {
    const config = `async function chart(data) {
      await Promise.resolve();
      return { title: { text: 'async' } };
    }`;
    const option = await buildChartOption(config, data, echartsStub);
    expect(option).toEqual({ title: { text: 'async' } });
  });

  test('resolves a Promise returned from a sync config', async () => {
    const config = `function chart(data) {
      return Promise.resolve({ title: { text: 'promised' } });
    }`;
    const option = await buildChartOption(config, data, echartsStub);
    expect(option).toEqual({ title: { text: 'promised' } });
  });

  test('rejects when the config defines no chart function', async () => {
    await expect(
      buildChartOption('const notChart = 1;', data, echartsStub),
    ).rejects.toThrow("chart config must define a function named 'chart'");
  });

  test('rejects when chart is not a function', async () => {
    await expect(
      buildChartOption('const chart = 42;', data, echartsStub),
    ).rejects.toThrow("chart config must define a function named 'chart'");
  });

  test('rejects when a sync config returns a non-object', async () => {
    await expect(
      buildChartOption('function chart() { return 42; }', data, echartsStub),
    ).rejects.toThrow('chart config must return an ECharts option object');
  });

  test('rejects when an async config resolves a non-object', async () => {
    const config = 'async function chart() { return null; }';
    await expect(buildChartOption(config, data, echartsStub)).rejects.toThrow(
      'chart config must return an ECharts option object',
    );
  });

  test('rejects with the config error when an async config throws', async () => {
    const config = `async function chart() {
      throw new Error('fetch failed');
    }`;
    await expect(buildChartOption(config, data, echartsStub)).rejects.toThrow(
      'fetch failed',
    );
  });

  test('evaluates the default chart config', async () => {
    // The starter config must always be valid, executable JavaScript that
    // produces an option (its JSDoc @type annotation is a comment at runtime).
    const timeSeries: ChartData = {
      columns: [
        { name: 'time', type: 'timestamptz' },
        { name: 'value', type: 'float8' },
      ],
      rows: [
        { time: '2026-01-01 00:00:00+00', value: 1.5 },
        { time: '2026-01-02 00:00:00+00', value: 2.5 },
      ],
    };
    const option = (await buildChartOption(
      DEFAULT_CHART_CONFIG,
      timeSeries,
      echartsStub,
    )) as { xAxis: { type: string }; series: { name: string }[] };
    expect(option.xAxis.type).toBe('time');
    expect(option.series.map((s) => s.name)).toEqual(['value']);
  });
});
