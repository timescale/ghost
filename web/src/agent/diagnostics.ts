import { loader } from '@monaco-editor/react';

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

// getChartConfigDiagnostics runs Monaco's JavaScript/TypeScript language service
// over a chart config and returns the same type and syntax errors the editor
// surfaces inline. It does this headlessly — creating a throwaway model so it
// works whether or not the config editor is currently mounted — using the same
// compiler options and injected EChartsOption/ChartData types as the editor
// (via configureMonacoForCharts). Suggestion-level diagnostics are excluded, to
// match the editor's squiggles. Throws if Monaco can't be loaded.
export async function getChartConfigDiagnostics(
  config: string,
): Promise<ChartConfigDiagnostic[]> {
  const monaco = await loader.init();
  await configureMonacoForCharts(monaco);

  const uri = monaco.Uri.parse(
    `file:///ghost-chart-config.check.${checkCounter++}.js`,
  );
  const model = monaco.editor.createModel(config, 'javascript', uri);
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
          const position = model.getPositionAt(d.start ?? 0);
          return {
            line: position.lineNumber,
            column: position.column,
            message: flattenMessage(d.messageText),
            severity: d.category === 1 ? 'error' : 'warning',
          } satisfies ChartConfigDiagnostic;
        })
    );
  } finally {
    model.dispose();
  }
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
  try {
    return await Promise.race([
      getChartConfigDiagnostics(config),
      new Promise<ChartConfigDiagnostic[]>((resolve) =>
        setTimeout(() => resolve([]), DIAGNOSTICS_TIMEOUT_MS),
      ),
    ]);
  } catch {
    return [];
  }
}
