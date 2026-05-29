import type React from 'react';
import {
  ContextMenuContext,
  ContextMenuProvider,
  ExecuteQueryEngine,
  QueryWidget,
  QueryWidgetProvider,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget';
import { useMemo, useRef, useState } from 'react';

import { countStatements } from '../lib/sql';

// Monaco's KeyMod/KeyCode constants are numeric and stable; hardcoding
// avoids dragging another copy of monaco-editor into this bundle just to
// reference the enum values.
const KEYMOD_CTRLCMD = 1 << 11; // 2048
const KEYMOD_WINCTRL = 1 << 8; // 256
const KEYCODE_ENTER = 3;

// preserveSelectionPlugin keeps the editor selection intact when the user
// fires the run-query keybinding. The widget's own `execute-query` action
// somehow loses the selection (cursor jumps to the document start), even
// though the equivalent button click does not. We register our own action
// for the same keybindings; Monaco prefers later-registered actions, so
// ours wins. We snapshot the selection, dispatch the widget's action via
// `trigger`, then restore the selection.
const preserveSelectionPlugin = {
  id: 'preserve-selection-on-run',
  init: ({ editor }: { editor: any }) => {
    const disposer = editor.addAction({
      id: 'ghost-execute-query-preserve-selection',
      label: 'Execute query (preserve selection)',
      keybindings: [
        KEYMOD_CTRLCMD | KEYCODE_ENTER,
        KEYMOD_WINCTRL | KEYCODE_ENTER,
      ],
      run: (ed: any) => {
        const selection = ed.getSelection();
        ed.trigger('keyboard', 'execute-query', null);
        if (selection && !selection.isEmpty()) {
          ed.setSelection(selection);
        }
      },
    });
    return [disposer];
  },
};

interface Props {
  projectId: string;
  databaseId: string;
  databaseName: string;
}

// QueryPanel renders the PopSQL query widget targeted at a single ghost
// database. The sessionKey is derived from the database ID so switching
// databases automatically invalidates the session (and tears down the
// in-process PG connection on the Go side).
export function QueryPanel({ projectId, databaseId, databaseName }: Props) {
  const [query, setQuery] = useState(`-- ${databaseName}\nSELECT 1;\n`);
  const [statementCount, setStatementCount] = useState(0);
  const lastRunSQLRef = useRef<string>('');
  // Plugin must be stable across renders so PopsqlEditor's initDeps don't
  // think the plugin set changed and re-create the editor.
  const editorPlugins = useMemo(() => [preserveSelectionPlugin] as any, []);

  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme="light">
        <ContextMenuProvider>
          <QueryWidget
            editorPlugins={editorPlugins}
            id={`ghost-${databaseId}`}
            query={query}
            onQueryChange={setQuery}
            sessionKey={`ghost-${databaseId}`}
            runSelection
            runButtonLabelWithSelection="Run selection"
            onQueryRun={({ query: executedSQL }) => {
              lastRunSQLRef.current = executedSQL;
            }}
            onQueryComplete={(args) => {
              // Only surface the counter when the run actually succeeded;
              // for errors / cancels, hide it to avoid implying anything
              // about what got committed.
              if ('rowsAffected' in args) {
                setStatementCount(countStatements(lastRunSQLRef.current));
              } else {
                setStatementCount(0);
              }
            }}
            renderToolbarAppendLeft={({ isRunning }) => {
              if (isRunning || statementCount <= 1) return null;
              return (
                <span className="ml-2 text-xs text-slate-500">
                  Executed {statementCount} statements
                </span>
              );
            }}
            getExecuteQueryData={({ runId, query }) => ({
              engine: ExecuteQueryEngine.timescaleQuery,
              params: {
                projectId,
                serviceId: databaseId,
                query,
                runId,
              },
            })}
          />
          <ContextMenuContext.Consumer>
            {({ render }: { render: () => React.ReactNode }) => render()}
          </ContextMenuContext.Consumer>
        </ContextMenuProvider>
      </QueryWidgetProvider>
    </TimescaleResultsCacheContextProvider>
  );
}
