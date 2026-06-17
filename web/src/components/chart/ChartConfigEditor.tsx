import Editor, {
  type BeforeMount,
  loader,
  type OnMount,
} from '@monaco-editor/react';
import { useCallback, useRef } from 'react';

import { DEFAULT_CHART_CONFIG } from './defaultConfig';
import { configureMonacoForCharts } from './monacoChartSetup';

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

  const handleMount = useCallback<OnMount>((editor) => {
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
  }, []);

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
