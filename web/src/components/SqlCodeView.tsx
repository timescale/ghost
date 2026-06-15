import {
  ContextMenuContext,
  ContextMenuProvider,
  type ExecuteQueryData,
  ExecuteQueryEngine,
  type GetExecuteQueryDataArgs,
  QueryWidget,
  QueryWidgetProvider,
  Theme,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget-cdn';
import type React from 'react';
import { useCallback } from 'react';

import { CopyButton } from './CopyButton';

interface Props {
  query: string;
  // Monaco language for the read-only editor. Defaults to the widget's SQL
  // language; pass 'plaintext' for prose content (e.g. object comments) that
  // shouldn't be SQL-highlighted.
  language?: string;
}

// SqlCodeView renders read-only, syntax-highlighted SQL using the PopSQL
// QueryWidget with the run button, results, status, and search all hidden — so
// only the code editor and a copy button (in the toolbar) are visible. Used to
// display object definitions (indexes, functions, procedures) with the same
// highlighting as the main query editor.
export function SqlCodeView({ query, language }: Props) {
  // Required by QueryWidget, but never invoked here: the editor is read-only
  // and the run button is hidden/disabled, so no query is ever executed.
  const getExecuteQueryData = useCallback(
    ({ runId }: GetExecuteQueryDataArgs): ExecuteQueryData => ({
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
      <QueryWidgetProvider theme={Theme.light}>
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
            editorLanguage={language}
            editorOptions={{
              minimap: { enabled: false },
              scrollBeyondLastLine: false,
              // Prose wraps to the modal width; SQL keeps Monaco's default
              // no-wrap + horizontal scroll.
              ...(language === 'plaintext' ? { wordWrap: 'on' as const } : {}),
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
