// JSDoc headers prepended to a chart config before type-checking, plus the
// logic that picks the right one. The chart config is plain JavaScript whose
// `chart` function is usually written without type annotations (e.g.
// agent-supplied configs), so TypeScript can't contextually type its
// parameters or check its return value against EChartsOption — the most common
// kind of chart-config mistake (a wrong key or value inside the returned
// option object) would then go completely unflagged. Prepending a JSDoc `@type`
// annotation gives the declaration the ChartFunction type (declared in
// monacoChartSetup.ts): `data` becomes ChartData, the optional second `echarts`
// parameter becomes the ECharts namespace, and the returned option literal is
// contextually checked against EChartsOption (the same red squiggles a human
// sees). Unlike a `@param`-based header, `@type` tolerates a chart function
// that declares fewer parameters than the type (e.g. just `chart(data)`), so
// configs that ignore the `echarts` argument aren't flagged.
//
// The header is prepended only for the diagnostics check, not for execution
// (buildChartOption runs the raw source).
//
// Each header is a single physical line so it doesn't shift the reported line
// numbers, which are mapped back to the user's source by subtracting the
// header's length from line-1 columns (see diagnostics.ts). A trailing space
// separates it from a leading `function` keyword on the config's first line.
//
// Known limitation (inherited from the previous `@param`-based header): the
// annotation binds to the config's first statement, and a user-authored JSDoc
// comment (`/** ... */`) directly above the function displaces it. Configs
// that carry their own JSDoc are expected to declare their own tags (as the
// default config does).
export const CONFIG_HEADER = '/** @type {ChartFunction} */ ';

// Header used when the chart function is declared `async`: TypeScript requires
// an async function's annotated return type to be exactly the global
// Promise<T> (error TS1065), so ChartFunction's sync|async return union can't
// be applied to it. AsyncChartFunction returns Promise<EChartsOption> only.
export const ASYNC_CONFIG_HEADER = '/** @type {AsyncChartFunction} */ ';

// Matches a chart function declared with `async` — either an
// `async function chart` declaration or a `chart = async ...` assignment
// (arrow or function expression). A textual heuristic, but the header can only
// bind to the config's first statement anyway, so configs are already
// conventionally shaped around a top-level `chart` definition.
const ASYNC_CHART_RE = /\basync\s+function\s+chart\b|\bchart\s*=\s*async\b/;

// configHeaderFor picks the type-checking header for a chart config: the
// async-typed header when `chart` is declared async, the sync|async union
// otherwise.
export function configHeaderFor(config: string): string {
  return ASYNC_CHART_RE.test(config) ? ASYNC_CONFIG_HEADER : CONFIG_HEADER;
}

// Matches a config whose first non-whitespace token is a JSDoc block comment.
// Such configs manage their own annotation: either that JSDoc carries typing
// tags (as the default config's does), or prepending our own annotation ahead
// of it would be displaced anyway (TypeScript binds only the closest JSDoc to
// a declaration).
const LEADING_JSDOC_RE = /^\s*\/\*\*/;

// ensureChartTypeAnnotation returns the config with a `@type` JSDoc annotation
// line prepended, unless it already starts with a JSDoc comment. Unlike the
// header prepended invisibly for diagnostics (configHeaderFor), this
// annotation becomes part of the stored source, so the *live* editor model is
// typed too: hover, completions, and squiggles see `data` as ChartData and
// `echarts` as the ECharts namespace instead of `any`. Applied to
// agent-supplied configs on arrival (see agent/dispatch.ts). Idempotent: the
// added line is itself a leading JSDoc, so a config that round-trips through
// the UI state and back isn't annotated twice.
export function ensureChartTypeAnnotation(config: string): string {
  if (LEADING_JSDOC_RE.test(config)) return config;
  return `${configHeaderFor(config).trimEnd()}\n${config}`;
}
