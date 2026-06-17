// The starter chart config shown the first time a user opens the chart editor.
// It defines a function `chart(data)` that returns an Apache ECharts option.
// The JSDoc `@param`/`@returns` annotations drive Monaco's type checking: the
// editor flags returns that don't satisfy `EChartsOption`.
//
// The default plots the first column on the x-axis and every numeric column as
// its own line series — a sensible starting point for time-series / sensor
// data. It also detects whether the x-axis column is time-like (by its column
// type or by sniffing values) and uses a 'time' axis if so. Edit it to suit
// your query.
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

  // Decide whether the x-axis is time-like, so we can use a 'time' axis
  // (which spaces points by their actual instant) instead of 'category'.
  const col0 = data.columns[0];
  const xType = col0?.type || '';
  // Postgres type names that represent a date/time.
  const timeTypeRe = /^(timestamp|timestamptz|date|time|timetz)/i;

  // Parse a value to epoch milliseconds, or NaN if it isn't a time. Numbers
  // must fall in a plausible ms-epoch range (~2001 to ~2286) so plain IDs
  // (1, 2, 3) and seconds-epoch values aren't mistaken for dates. Strings try
  // Date.parse first, then retry with Postgres's timestamp format normalized
  // to ISO 8601 — e.g. '2026-05-22 20:56:07.782576+00' becomes
  // '2026-05-22T20:56:07.782+00:00' (space to 'T', microseconds trimmed to
  // milliseconds, '+00' offset expanded to '+00:00').
  const MS_EPOCH_MIN = 1e12;
  const MS_EPOCH_MAX = 1e13;
  const parseTime = (v) => {
    if (v instanceof Date) return v.getTime();
    if (typeof v === 'number')
      return v >= MS_EPOCH_MIN && v <= MS_EPOCH_MAX ? v : NaN;
    if (typeof v !== 'string' || v === '') return NaN;
    const direct = Date.parse(v);
    if (!Number.isNaN(direct)) return direct;
    const iso = v
      .replace(' ', 'T')
      .replace(/(\\.\\d{3})\\d+/, '$1')
      .replace(/([+-]\\d{2})$/, '$1:00');
    return Date.parse(iso);
  };

  // Decide whether the x-axis is time-like and, in the same pass, normalize
  // its values to epoch ms — ECharts' time axis can't parse Postgres timestamp
  // strings. Each converted row is a shallow copy with the x field replaced;
  // null x values and the other columns pass through untouched.
  //
  // If the column type names a date/time, trust it. Otherwise sniff: every
  // non-null x must parse as a time, with at least one non-null value.
  // parseTime only accepts ms-range numbers, so non-time columns fall through
  // to a 'category' axis, and the loop bails on the first non-time value.
  let xIsTime = timeTypeRe.test(xType);
  let source = data.rows;
  if (xIsTime) {
    source = data.rows.map((row) =>
      row[x] == null ? row : { ...row, [x]: parseTime(row[x]) },
    );
  } else if (data.rows.length) {
    source = new Array(data.rows.length);
    let allTime = true;
    let sawTimeValue = false;
    for (let i = 0; i < data.rows.length; i++) {
      const row = data.rows[i];
      if (row[x] == null) {
        source[i] = row;
        continue;
      }
      const parsed = parseTime(row[x]);
      if (Number.isNaN(parsed)) {
        allTime = false;
        break;
      }
      source[i] = { ...row, [x]: parsed };
      sawTimeValue = true;
    }
    xIsTime = allTime && sawTimeValue;
    if (!xIsTime) {
      source = data.rows;
    }
  }

  return {
    tooltip: { trigger: 'axis' },
    legend: {},
    grid: { left: 56, right: 24, top: 48, bottom: 56, containLabel: true },
    dataset: { source },
    xAxis: { type: xIsTime ? 'time' : 'category' },
    yAxis: { type: 'value', scale: true },
    series: yColumns.map((y) => ({
      type: 'line',
      name: y,
      encode: { x, y },
    })),
  };
}
`;
