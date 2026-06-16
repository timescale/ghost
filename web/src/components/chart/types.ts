// ChartData is the shape passed as the single `data` argument to the
// user-authored chart config function. It mirrors a SQL result set: an ordered
// list of columns plus the rows as objects keyed by column name.
export interface ChartColumn {
  name: string;
  // Postgres type name (e.g. 'int4', 'timestamptz'), when known.
  type?: string;
}

export interface ChartData {
  rows: Record<string, unknown>[];
  columns: ChartColumn[];
}

// ResultView selects what's shown below the query editor: the results table,
// the rendered chart, or the chart config editor. Defined here (free of any
// component imports) so non-UI modules like the store can reference it without
// pulling in React components.
export type ResultView = 'table' | 'chart' | 'editor';
