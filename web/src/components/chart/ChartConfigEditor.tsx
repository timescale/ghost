import Editor, { loader, useMonaco } from '@monaco-editor/react';
import { useEffect } from 'react';

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
  const monaco = useMonaco();

  useEffect(() => {
    if (!monaco) return;
    configureMonacoForCharts(monaco).catch(console.error);
  }, [monaco]);

  return (
    <Editor
      language="javascript"
      // Stable model path so the language service treats edits as one file.
      path="ghost-chart-config.js"
      theme="vs"
      value={config}
      onChange={(value) => onChange(value ?? '')}
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
