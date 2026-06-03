import {
  ContextMenuContext,
  ContextMenuProvider,
  ExecuteQueryEngine,
  QueryWidget,
  QueryWidgetProvider,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget-cdn';
import type React from 'react';
import { useCallback, useState } from 'react';

interface Props {
  projectId: string;
  databaseId: string;
  databaseName: string;
  query: string;
  onQueryChange: (next: string) => void;
  editorHeight: number;
  onResizeEditor: (height: number) => void;
}

// QueryPanel renders the PopSQL query widget targeted at a single ghost
// database. The sessionKey is derived from the database ID so switching
// databases automatically invalidates the session (and tears down the
// in-process PG connection on the Go side).
export function QueryPanel({
  projectId,
  databaseId,
  databaseName: _databaseName,
  query,
  onQueryChange,
  editorHeight,
  onResizeEditor,
}: Props) {
  const [statementCount, setStatementCount] = useState(0);

  const handleQueryComplete = useCallback(
    (args: { statementCount?: number }) => {
      setStatementCount(args.statementCount ?? 0);
    },
    [],
  );

  const renderToolbarAppendLeft = useCallback(
    ({ isRunning }: { isRunning: boolean }) => {
      if (isRunning || statementCount <= 1) return null;
      return (
        <span className="ml-2 text-xs text-slate-500">
          Executed {statementCount} statements
        </span>
      );
    },
    [statementCount],
  );

  const getExecuteQueryData = useCallback(
    ({ runId, query: q }: { runId: string; query: string }) => ({
      engine: ExecuteQueryEngine.timescaleQuery,
      params: {
        projectId,
        serviceId: databaseId,
        query: q,
        runId,
      },
    }),
    [projectId, databaseId],
  );

  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme="light">
        <ContextMenuProvider>
          <QueryWidget
            className="flex-auto"
            resizeHandles="split"
            editorMinHeight={200}
            editorHeight={editorHeight}
            onResizeEditor={onResizeEditor}
            id={databaseId}
            query={query}
            onQueryChange={onQueryChange}
            sessionKey={databaseId}
            runSelection
            runButtonLabelWithSelection="Run selection"
            onQueryComplete={handleQueryComplete}
            renderToolbarAppendLeft={renderToolbarAppendLeft}
            getExecuteQueryData={getExecuteQueryData}
          />
          <ContextMenuContext.Consumer>
            {({ render }: { render: () => React.ReactNode }) => render()}
          </ContextMenuContext.Consumer>
        </ContextMenuProvider>
      </QueryWidgetProvider>
    </TimescaleResultsCacheContextProvider>
  );
}
