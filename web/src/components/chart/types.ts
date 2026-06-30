// ChartData is the shape passed as the single `data` argument to the
// user-authored chart config function. It mirrors a SQL result set: an ordered
// list of columns plus the rows as objects keyed by column name.
export interface ChartColumn {
  name: string;
  // Postgres type name (e.g. 'int4', 'timestamptz'), when known.
  type?: string;
}

export interface ChartData {
  // Values are typed as `any` (not `unknown`) so chart configs can use a column
  // directly as ECharts axis data or in arithmetic without a type error on
  // every access. This must stay in sync with the ambient `ChartData` fed to
  // Monaco in monacoChartSetup.ts, which is what governs editor diagnostics.
  // biome-ignore lint/suspicious/noExplicitAny: intentional, see comment above.
  rows: Record<string, any>[];
  columns: ChartColumn[];
}

// ResultView selects what's shown below the query editor: the results table,
// the rendered chart, or the chart config editor. Defined here (free of any
// component imports) so non-UI modules like the store can reference it without
// pulling in React components.
export type ResultView = 'table' | 'chart' | 'chart_editor';
