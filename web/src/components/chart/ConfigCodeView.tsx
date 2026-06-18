import Editor, { loader } from '@monaco-editor/react';

// Configure to load monaco-editor from CDN (same source as the editable
// ChartConfigEditor).
loader.config({
  paths: { vs: 'https://cdn.jsdelivr.net/npm/monaco-editor@0.55.1/min/vs' },
});

interface Props {
  config: string;
}

// ConfigCodeView renders a chart config as read-only, syntax-highlighted
// JavaScript. Used in the chart history modal to display a historical config
// without the full editing/type-checking machinery of ChartConfigEditor.
export function ConfigCodeView({ config }: Props) {
  return (
    <Editor
      language="javascript"
      theme="vs"
      value={config}
      loading={
        <div className="p-3 text-xs text-slate-500">Loading editor…</div>
      }
      options={{
        readOnly: true,
        domReadOnly: true,
        minimap: { enabled: false },
        fontSize: 12,
        tabSize: 2,
        scrollBeyondLastLine: false,
        automaticLayout: true,
        wordWrap: 'on',
        padding: { top: 8, bottom: 8 },
        fixedOverflowWidgets: true,
      }}
    />
  );
}
