import type { Monaco } from '@monaco-editor/react';

// The echarts version whose bundled type declarations we feed to Monaco's
// TypeScript service. Keep this in sync with the runtime echarts <script> tag
// in index.html so the editor's types match what actually renders.
const ECHARTS_VERSION = '6.1.0';
const ECHARTS_TYPES_URL = `https://cdn.jsdelivr.net/npm/echarts@${ECHARTS_VERSION}/types/dist/echarts.d.ts`;

// Ambient declarations injected into the editor's language service so the chart
// config can reference `ChartData` and `EChartsOption` without imports. This is
// a module (note `export {}`), so `declare global` is required to make the
// names global. `EChartsOption` is aliased from the echarts types lib added
// below.
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
    /** Result rows as objects keyed by column name. */
    rows: Record<string, unknown>[];
    /** Ordered column metadata. */
    columns: ChartColumn[];
  }
  /** Apache ECharts option object (from echarts ${ECHARTS_VERSION}). */
  type EChartsOption = import('echarts').EChartsOption;
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
      noSemanticValidation: false,
      noSyntaxValidation: false,
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
