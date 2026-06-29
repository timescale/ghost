import Editor, {
  type BeforeMount,
  loader,
  type Monaco,
  type OnMount,
} from '@monaco-editor/react';
import type { editor } from 'monaco-editor';
import { useCallback, useEffect, useRef } from 'react';

import { getChartConfigMarkers } from '../../agent/diagnostics';
import { DEFAULT_CHART_CONFIG } from './defaultConfig';
import { configureMonacoForCharts } from './monacoChartSetup';

// Marker owner string namespacing the squiggles this editor paints, so
// setModelMarkers replaces only our markers and never clobbers any other
// owner's.
const MARKER_OWNER = 'ghost-chart-config';

// Configure to load monaco-editor from CDN
loader.config({
  paths: { vs: 'https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min/vs' },
});

interface Props {
  config: string;
  onChange: (next: string) => void;
}

// ChartConfigEditor is a Monaco editor for the chart config. It runs as
// JavaScript with `checkJs`, and (via configureMonacoForCharts) is fed the
// echarts type bundle plus ambient `ChartData`/`EChartsOption` types — so the
// JSDoc-annotated `chart(data)` function is type-checked against EChartsOption,
// surfacing return-type errors inline.
export function ChartConfigEditor({ config, onChange }: Props) {
  // Configure the JS language service before the editor model is created.
  const handleBeforeMount = useCallback<BeforeMount>((monaco) => {
    configureMonacoForCharts(monaco).catch(console.error);
  }, []);

  // Keep onChange in a ref so the context-menu action (registered once on
  // mount) always calls the latest handler rather than a stale closure.
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;

  // The editor and monaco instances, captured on mount, so we can recompute
  // markers whenever the config changes (including agent-pushed configs, which
  // arrive as a new `config` prop without a user keystroke).
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const monacoRef = useRef<Monaco | null>(null);

  // Paint type/syntax squiggles ourselves. Automatic in-model validation is
  // disabled (see monacoChartSetup) because the config usually lacks the JSDoc
  // that types `data`/the return value; getChartConfigMarkers type-checks the
  // header-augmented source instead — the exact same check the headless agent
  // path runs — so the human sees the same squiggles the agent receives.
  // `version` guards against a slow worker result clobbering a newer one: each
  // call captures the current version and bails if it changed while awaiting.
  const markerVersionRef = useRef(0);
  const paintMarkers = useCallback((source: string) => {
    const monaco = monacoRef.current;
    const editor = editorRef.current;
    if (!monaco || !editor) return;
    const version = ++markerVersionRef.current;
    getChartConfigMarkers(monaco, source)
      .then((markers) => {
        // The editor may have unmounted, or `config` changed again, while we
        // awaited the worker — don't paint stale markers in either case.
        if (version !== markerVersionRef.current) return;
        const model = editor.getModel();
        if (!model) return;
        monaco.editor.setModelMarkers(
          model,
          MARKER_OWNER,
          markers.map((m) => ({
            ...m,
            severity:
              m.severity === 'error'
                ? monaco.MarkerSeverity.Error
                : monaco.MarkerSeverity.Warning,
          })),
        );
      })
      .catch(() => {
        // Best effort: if Monaco's worker can't be reached, just skip squiggles.
      });
  }, []);

  const handleMount = useCallback<OnMount>(
    (editor, monaco) => {
      editorRef.current = editor;
      monacoRef.current = monaco;
      // Add a "Reset to default" entry to the editor's right-click context menu.
      editor.addAction({
        id: 'ghost.resetChartConfig',
        label: 'Reset chart config to default',
        contextMenuGroupId: 'modification',
        run: () => {
          editor.setValue(DEFAULT_CHART_CONFIG);
          onChangeRef.current(DEFAULT_CHART_CONFIG);
        },
      });
      // Paint markers for the initial config: the effect below runs once before
      // the editor finishes mounting (refs still null), and won't re-run until
      // `config` next changes, so the first config would otherwise go unmarked.
      paintMarkers(editor.getValue());
    },
    [paintMarkers],
  );

  // Recompute markers whenever `config` changes — user edits and agent-pushed
  // configs alike. (Skips the first run before mount, when the refs are null;
  // handleMount paints the initial config.)
  useEffect(() => {
    paintMarkers(config);
  }, [config, paintMarkers]);

  return (
    <Editor
      language="javascript"
      // Stable model path so the language service treats edits as one file.
      path="ghost-chart-config.js"
      theme="vs"
      value={config}
      onChange={(value) => onChange(value ?? '')}
      beforeMount={handleBeforeMount}
      onMount={handleMount}
      loading={
        <div className="p-3 text-xs text-slate-500">Loading editor…</div>
      }
      options={{
        minimap: { enabled: false },
        fontSize: 12,
        tabSize: 2,
        scrollBeyondLastLine: false,
        automaticLayout: true,
        wordWrap: 'on',
        padding: { top: 8, bottom: 8 },
        // Render hover/suggest widgets at the document body so they aren't
        // clipped by the editor pane's overflow-hidden / rounded border.
        fixedOverflowWidgets: true,
      }}
    />
  );
}
