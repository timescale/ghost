import type { Monaco } from '@monaco-editor/react';

// The echarts version whose bundled type declarations we feed to Monaco's
// TypeScript service. Keep this in sync with the runtime echarts <script> tag
// in index.html so the editor's types match what actually renders.
const ECHARTS_VERSION = '6.1.0';
const ECHARTS_TYPES_URL = `https://cdn.jsdelivr.net/npm/echarts@${ECHARTS_VERSION}/types/dist/echarts.d.ts`;

// Ambient declarations injected into the editor's language service so the chart
// config can reference `ChartData`, `EChartsOption`, and the `echarts` global
// without imports. This is a module (note `export {}`), so `declare global` is
// required to make the names global. `EChartsOption` and `EChartsNamespace`
// are aliased from the echarts types lib added below.
const GLOBALS_DTS = `export {};
declare global {
  interface ChartColumn {
    /** Column name. */
    name: string;
    /** Postgres type name (e.g. 'int4', 'timestamptz'), when known. */
    type?: string;
  }
  /** The query result passed to the chart function. */
  interface ChartData {
    /**
     * Result rows as objects keyed by column name. Values are typed as 'any'
     * (not 'unknown') so a config can use a column directly as ECharts axis
     * data or in arithmetic (e.g. data.rows.map((r) => r.year)) without a type
     * error on every access. The valuable check -- the returned EChartsOption
     * shape -- is preserved via the @type annotation in the config header (see
     * configHeader.ts).
     */
    rows: Record<string, any>[];
    /** Ordered column metadata. */
    columns: ChartColumn[];
  }
  /** Apache ECharts option object (from echarts ${ECHARTS_VERSION}). */
  type EChartsOption = import('echarts').EChartsOption;
  /**
   * The Apache ECharts namespace (from echarts ${ECHARTS_VERSION}), e.g. for
   * echarts.registerMap(...). Available as the global 'echarts' and also
   * passed to the chart function as its second argument.
   */
  type EChartsNamespace = typeof import('echarts');
  /** The Apache ECharts global (loaded from the CDN). */
  const echarts: EChartsNamespace;
  /**
   * The chart config's 'chart' function: builds an ECharts option from the
   * query results. It may return the option directly or a Promise of it (e.g.
   * when it must fetch map GeoJSON before building the option), and may
   * declare fewer parameters than the type (e.g. just 'chart(data)').
   */
  type ChartFunction = (
    data: ChartData,
    echarts: EChartsNamespace,
  ) => EChartsOption | Promise<EChartsOption>;
  /**
   * ChartFunction variant applied to 'async function chart' declarations:
   * TypeScript requires an async function's annotated return type to be
   * exactly the global Promise<T> (never a union), so ChartFunction's
   * sync|async return union can't be used there.
   */
  type AsyncChartFunction = (
    data: ChartData,
    echarts: EChartsNamespace,
  ) => Promise<EChartsOption>;
}
`;

let echartsTypesPromise: Promise<string> | null = null;

// Fetch (and cache) the bundled echarts type declarations from the CDN. The
// browser's HTTP cache makes repeat loads cheap; the module-level promise
// dedupes concurrent callers within a session.
function fetchEchartsTypes(): Promise<string> {
  echartsTypesPromise ??= fetch(ECHARTS_TYPES_URL).then((res) => {
    if (!res.ok) {
      throw new Error(`failed to load echarts types: ${res.status}`);
    }
    return res.text();
  });
  return echartsTypesPromise;
}

let configurePromise: Promise<void> | null = null;

// configureMonacoForCharts wires up the JS language service so the chart config
// editor gets EChartsOption-aware type checking: lenient compiler options
// (so ordinary JS isn't over-flagged) plus `checkJs` for JSDoc-driven type
// errors, the echarts type bundle, and our ambient globals. Runs once per page.
export function configureMonacoForCharts(monaco: Monaco): Promise<void> {
  configurePromise ??= (async () => {
    const ts = monaco.languages.typescript;
    ts.javascriptDefaults.setCompilerOptions({
      target: ts.ScriptTarget.ESNext,
      module: ts.ModuleKind.ESNext,
      moduleResolution: ts.ModuleResolutionKind.NodeJs,
      allowJs: true,
      checkJs: true,
      allowNonTsExtensions: true,
      lib: ['esnext', 'dom'],
    });
    ts.javascriptDefaults.setDiagnosticsOptions({
      // Turn OFF Monaco's automatic in-model validation. The chart config is
      // plain JS whose `chart(data)` function is usually written WITHOUT the
      // JSDoc that types `data`/the return value (e.g. agent-supplied configs),
      // so validating the model text as-is would either miss the EChartsOption
      // type errors entirely or, worse, double-mark against the wrong source.
      // Instead the editor paints markers itself via getChartConfigMarkers,
      // which type-checks the header-augmented source — identical to the
      // headless agent path — so the human sees exactly the squiggles the agent
      // receives. The TS worker the headless path drives is unaffected by these
      // flags (it calls getSemantic/SyntacticDiagnostics directly).
      noSemanticValidation: true,
      noSyntaxValidation: true,
      // Suggestions (e.g. "this could be a const") would be noisy here.
      noSuggestionDiagnostics: true,
    });

    const echartsTypes = await fetchEchartsTypes();
    ts.javascriptDefaults.addExtraLib(
      echartsTypes,
      'file:///node_modules/echarts/index.d.ts',
    );
    ts.javascriptDefaults.addExtraLib(
      GLOBALS_DTS,
      'file:///ghost-chart-globals.d.ts',
    );
  })();
  return configurePromise;
}
