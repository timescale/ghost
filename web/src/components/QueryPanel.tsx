import { useQueryClient } from '@tanstack/react-query';
import {
  ContextMenuContext,
  ContextMenuProvider,
  type ExecuteQueryData,
  ExecuteQueryEngine,
  type GetExecuteQueryDataArgs,
  type OnQueryCompleteArgs,
  QueryWidget,
  QueryWidgetProvider,
  Theme,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget-cdn';
import type React from 'react';
import { useCallback, useMemo, useState } from 'react';

import { useAutocompletePlugin } from '../autocomplete/useAutocompletePlugin';

// Postgres command tags whose execution can change the schema; a successful
// statement with one of these prefixes refreshes the schema cache so the tree
// and autocomplete pick up the change. Plain SELECT/INSERT/UPDATE/DELETE don't
// alter the schema, so they don't trigger a (relatively expensive) refetch.
const DDL_COMMAND_PREFIXES = [
  'CREATE',
  'ALTER',
  'DROP',
  'TRUNCATE',
  'COMMENT',
  'GRANT',
  'REVOKE',
  'RENAME',
  'REINDEX',
];

function isSchemaChangingCommand(command: string | undefined): boolean {
  if (!command) return false;
  const verb = command.trim().toUpperCase();
  return DDL_COMMAND_PREFIXES.some((prefix) => verb.startsWith(prefix));
}

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
  const queryClient = useQueryClient();

  const autocompletePlugin = useAutocompletePlugin(databaseId);
  const editorPlugins = useMemo(
    () => [autocompletePlugin],
    [autocompletePlugin],
  );

  const handleQueryComplete = useCallback(
    (args: OnQueryCompleteArgs) => {
      setStatementCount(args.statementCount ?? 0);
      // Refresh the schema (shared by the tree and autocomplete) after a
      // statement that may have altered it, so new objects appear without a
      // manual refresh.
      if (isSchemaChangingCommand(args.command)) {
        void queryClient.invalidateQueries({
          queryKey: ['schema', databaseId],
        });
      }
    },
    [queryClient, databaseId],
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
    ({ runId, query: q }: GetExecuteQueryDataArgs): ExecuteQueryData => ({
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
      <QueryWidgetProvider theme={Theme.light}>
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
            editorPlugins={editorPlugins}
          />
          <ContextMenuContext.Consumer>
            {({ render }: { render: () => React.ReactNode }) => render()}
          </ContextMenuContext.Consumer>
        </ContextMenuProvider>
      </QueryWidgetProvider>
    </TimescaleResultsCacheContextProvider>
  );
}
