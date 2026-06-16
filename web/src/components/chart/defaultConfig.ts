// The starter chart config shown the first time a user opens the chart editor.
// It defines a function `chart(data)` that returns an Apache ECharts option.
// The JSDoc `@param`/`@returns` annotations drive Monaco's type checking: the
// editor flags returns that don't satisfy `EChartsOption`.
//
// The default plots the first column on the x-axis and every numeric column as
// its own line series — a sensible starting point for time-series / sensor
// data. Edit it to suit your query.
export const DEFAULT_CHART_CONFIG = `/**
 * Build an Apache ECharts option from the query results.
 *
 * \`data\` provides:
 *   data.rows    – the result rows as objects keyed by column name, e.g.
 *                  [{ time: '2024-01-01', temperature: 21.5, humidity: 40 }, ...]
 *   data.columns – ordered column metadata: [{ name, type }, ...]
 *
 * How the data reaches the chart: we hand the rows to ECharts as a
 * \`dataset.source\`. Each series then maps columns to axes with \`encode\`
 * (e.g. \`encode: { x: 'time', y: 'temperature' }\`), so ECharts reads those
 * fields out of every row. No manual reshaping needed for the common case —
 * but you can transform \`data.rows\` yourself before returning if you prefer.
 *
 * See https://echarts.apache.org/en/option.html
 *
 * @param {ChartData} data
 * @returns {EChartsOption}
 */
function chart(data) {
  // Use the first column for the x-axis (often a timestamp or label)
  // and plot every other column that holds numbers as its own line.
  const [x, ...rest] = data.columns.map((c) => c.name);
  const yColumns = rest.filter(
    (name) => data.rows.some((row) => typeof row[name] === 'number'),
  );

  return {
    tooltip: { trigger: 'axis' },
    legend: {},
    grid: { left: 56, right: 24, top: 48, bottom: 56, containLabel: true },
    dataset: { source: data.rows },
    xAxis: { type: 'category' },
    yAxis: { type: 'value', scale: true },
    series: yColumns.map((y) => ({
      type: 'line',
      name: y,
      encode: { x, y },
    })),
  };
}
`;
