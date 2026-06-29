import { loader, type Monaco } from '@monaco-editor/react';

import { configureMonacoForCharts } from '../components/chart/monacoChartSetup';
import { type DiagnosticMessageChain, flattenMessage } from './flattenMessage';

// Configure the monaco loader to use the same CDN as the editors, so headless
// diagnostics work even when no editor component has mounted yet.
loader.config({
  paths: { vs: 'https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min/vs' },
});

// A single type/syntax issue reported by Monaco's TypeScript language service
// for a chart config — the same errors the editor shows as red squiggles.
export interface ChartConfigDiagnostic {
  // 1-based line and column of the issue in the config source.
  line: number;
  column: number;
  message: string;
  severity: 'error' | 'warning';
}

// The subset of a TypeScript diagnostic the worker returns (it omits `file`).
interface WorkerDiagnostic {
  start?: number;
  length?: number;
  messageText: string | DiagnosticMessageChain;
  // 0 = warning, 1 = error, 2 = suggestion, 3 = message.
  category: number;
}

// Unique-per-call model URI so a temporary model never collides with the live
// editor's model (or a previous check's), which Monaco forbids.
let checkCounter = 0;

// JSDoc header prepended to every config before type-checking. The chart config
// is plain JavaScript whose `chart(data)` function is usually written without
// type annotations, so TypeScript can't contextually type its `data` parameter
// or check its return value against EChartsOption — the most common kind of
// chart-config mistake (a wrong key or value inside the returned option object)
// would then go completely unflagged. Annotating the function via JSDoc gives
// `data` the ChartData type and the return value the EChartsOption type, so the
// language service checks the whole returned option literal against the ECharts
// schema (the same red squiggles a human sees). It's prepended only for the
// diagnostics check, not for execution (buildChartOption runs the raw source).
//
// It's a single physical line so it doesn't shift the reported line numbers,
// which are mapped back to the user's source by subtracting the prepended
// columns — see CONFIG_HEADER_OFFSET. A trailing space separates it from a
// leading `function` keyword on the user's first line.
// JSDoc header prepended to every config before type-checking — see the long
// note above. Exported so the live editor and the headless agent path stay in
// lockstep (both must type-check the identical augmented source).
export const CONFIG_HEADER =
  '/** @param {ChartData} data @returns {EChartsOption} */ ';
// Number of characters the header adds to the first line, so positions on line
// 1 can be shifted back to the user's original columns. Since the header has no
// newline, only line 1's columns are offset; all later lines are unchanged.
const CONFIG_HEADER_OFFSET = CONFIG_HEADER.length;

// userColumn maps a column from the header-augmented source back to the user's
// original source. The header is prepended to line 1 only (no newline), so
// subtract its width from columns on line 1; later lines are unaffected. Clamp
// to 1 so a position landing inside the header (which shouldn't happen for a
// valid comment) still reports a sane column.
function userColumn(lineNumber: number, column: number): number {
  return lineNumber === 1 ? Math.max(1, column - CONFIG_HEADER_OFFSET) : column;
}

// A diagnostic mapped back to the user's source with a full range, so the live
// editor can paint a squiggle over the exact offending span (not just a point).
export interface ChartConfigMarker {
  startLineNumber: number;
  startColumn: number;
  endLineNumber: number;
  endColumn: number;
  message: string;
  severity: 'error' | 'warning';
}

// runChartConfigDiagnostics type-checks a chart config against the
// header-augmented source on a throwaway model and returns the issues mapped
// back to the user's source (full ranges). It's the single source of truth
// shared by the headless agent path (getChartConfigDiagnostics) and the live
// editor's marker painting (getChartConfigMarkers), so both surface identical
// issues. `monaco` must already be configured via configureMonacoForCharts.
async function runChartConfigDiagnostics(
  monaco: Monaco,
  config: string,
): Promise<ChartConfigMarker[]> {
  const uri = monaco.Uri.parse(
    `file:///ghost-chart-config.check.${checkCounter++}.js`,
  );
  const model = monaco.editor.createModel(
    CONFIG_HEADER + config,
    'javascript',
    uri,
  );
  try {
    const getWorker = await monaco.languages.typescript.getJavaScriptWorker();
    const worker = await getWorker(uri);
    const fileName = uri.toString();
    const [syntactic, semantic] = (await Promise.all([
      worker.getSyntacticDiagnostics(fileName),
      worker.getSemanticDiagnostics(fileName),
    ])) as [WorkerDiagnostic[], WorkerDiagnostic[]];

    return (
      [...syntactic, ...semantic]
        // Keep only errors (1) and warnings (0); drop suggestions/messages.
        .filter((d) => d.category === 0 || d.category === 1)
        .map((d) => {
          const offset = d.start ?? 0;
          const start = model.getPositionAt(offset);
          const end = model.getPositionAt(offset + (d.length ?? 0));
          return {
            startLineNumber: start.lineNumber,
            startColumn: userColumn(start.lineNumber, start.column),
            endLineNumber: end.lineNumber,
            endColumn: userColumn(end.lineNumber, end.column),
            message: flattenMessage(d.messageText),
            severity: d.category === 1 ? 'error' : 'warning',
          } satisfies ChartConfigMarker;
        })
    );
  } finally {
    model.dispose();
  }
}

// getChartConfigDiagnostics runs Monaco's JavaScript/TypeScript language service
// over a chart config and returns the same type and syntax errors the editor
// surfaces inline. It does this headlessly — loading and configuring Monaco,
// then type-checking on a throwaway model so it works whether or not the config
// editor is currently mounted. Suggestion-level diagnostics are excluded, to
// match the editor's squiggles. Throws if Monaco can't be loaded. The
// agent-facing shape reports only the start position of each issue.
export async function getChartConfigDiagnostics(
  config: string,
): Promise<ChartConfigDiagnostic[]> {
  const monaco = await loader.init();
  await configureMonacoForCharts(monaco);
  const markers = await runChartConfigDiagnostics(monaco, config);
  return markers.map((m) => ({
    line: m.startLineNumber,
    column: m.startColumn,
    message: m.message,
    severity: m.severity,
  }));
}

// getChartConfigMarkers is the live editor's counterpart to
// getChartConfigDiagnostics: given an already-loaded `monaco`, it type-checks
// the config against the same header-augmented source and returns full-range
// markers the editor can paint onto its model. This is what lets the human see
// the same red squiggles the agent receives, even for configs (e.g.
// agent-supplied ones) that omit the JSDoc annotation themselves.
export async function getChartConfigMarkers(
  monaco: Monaco,
  config: string,
): Promise<ChartConfigMarker[]> {
  await configureMonacoForCharts(monaco);
  return runChartConfigDiagnostics(monaco, config);
}

// Cap how long we wait for diagnostics. Loading Monaco from the CDN and warming
// its TS worker is usually fast, but must never stall the agent tool call (e.g.
// CDN unreachable, or a test environment with no real DOM). On timeout we just
// return no diagnostics.
const DIAGNOSTICS_TIMEOUT_MS = 5000;

// tryGetChartConfigDiagnostics is a best-effort wrapper that never throws and
// never hangs: failing to load Monaco (e.g. offline, or a test environment)
// or exceeding the timeout must not fail the agent tool call. Returns [] when
// diagnostics can't be computed in time.
export async function tryGetChartConfigDiagnostics(
  config: string,
): Promise<ChartConfigDiagnostic[]> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      getChartConfigDiagnostics(config),
      new Promise<ChartConfigDiagnostic[]>((resolve) => {
        timer = setTimeout(() => resolve([]), DIAGNOSTICS_TIMEOUT_MS);
      }),
    ]);
  } catch {
    return [];
  } finally {
    // Clear the timeout once the race settles so a fast diagnostics result
    // doesn't leave a 5s timer (and its microtask) dangling per call.
    clearTimeout(timer);
  }
}
