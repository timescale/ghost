import {
  ContextMenuContext,
  ContextMenuProvider,
  ExecuteQueryEngine,
  QueryWidget,
  QueryWidgetProvider,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget-cdn';
import type React from 'react';
import { useCallback, useEffect, useRef, useState } from 'react';

import { Icon } from './Icon';

interface Props {
  query: string;
}

// SqlCodeView renders read-only, syntax-highlighted SQL using the PopSQL
// QueryWidget with the run button, results, status, and search all hidden — so
// only the code editor and a copy button (in the toolbar) are visible. Used to
// display object definitions (indexes, functions, procedures) with the same
// highlighting as the main query editor.
export function SqlCodeView({ query }: Props) {
  // Required by QueryWidget, but never invoked here: the editor is read-only
  // and the run button is hidden/disabled, so no query is ever executed.
  const getExecuteQueryData = useCallback(
    ({ runId }: { runId: string }) => ({
      engine: ExecuteQueryEngine.timescaleQuery,
      params: { projectId: '', serviceId: '', query, runId },
    }),
    [query],
  );

  const renderToolbarAppendRight = useCallback(
    () => <CopyButton text={query} />,
    [query],
  );

  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme="light">
        <ContextMenuProvider>
          <QueryWidget
            id="definition-viewer"
            query={query}
            getExecuteQueryData={getExecuteQueryData}
            readonlyEditor
            disableRun
            hideRunButton
            hideResults
            hideSessionStatus
            hideSearchInput
            resizeHandles="none"
            renderToolbarAppendRight={renderToolbarAppendRight}
            editorOptions={{
              minimap: { enabled: false },
              scrollBeyondLastLine: false,
            }}
          />
          <ContextMenuContext.Consumer>
            {({ render }: { render: () => React.ReactNode }) => render()}
          </ContextMenuContext.Consumer>
        </ContextMenuProvider>
      </QueryWidgetProvider>
    </TimescaleResultsCacheContextProvider>
  );
}

// CopyButton copies the given text to the clipboard and briefly animates to a
// green checkmark for feedback. Rendered inside the QueryWidget toolbar.
function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (resetTimer.current) clearTimeout(resetTimer.current);
    },
    [],
  );

  const onCopy = () => {
    void navigator.clipboard.writeText(text);
    setCopied(true);
    if (resetTimer.current) clearTimeout(resetTimer.current);
    resetTimer.current = setTimeout(() => setCopied(false), 1500);
  };

  return (
    <button
      type="button"
      onClick={onCopy}
      aria-label={copied ? 'Copied' : 'Copy to clipboard'}
      title={copied ? 'Copied' : 'Copy to clipboard'}
      className={`rounded border p-1.5 transition-colors ${
        copied
          ? 'border-green-300 bg-green-50 text-green-600'
          : 'border-slate-300 bg-white text-slate-600 hover:bg-slate-50 hover:text-slate-800'
      }`}
    >
      <span className="relative block size-4">
        <Icon
          name="copy"
          size={16}
          className={`absolute inset-0 transition-all duration-200 ${
            copied ? 'scale-50 opacity-0' : 'scale-100 opacity-100'
          }`}
        />
        <Icon
          name="check"
          size={16}
          className={`absolute inset-0 transition-all duration-200 ${
            copied ? 'scale-100 opacity-100' : 'scale-50 opacity-0'
          }`}
        />
      </span>
    </button>
  );
}
