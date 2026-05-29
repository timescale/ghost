import type React from 'react';
import {
  ContextMenuContext,
  ContextMenuProvider,
  ExecuteQueryEngine,
  QueryWidget,
  QueryWidgetProvider,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget';
import { useRef, useState } from 'react';

import { countStatements } from '../lib/sql';

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

  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme="light">
        <ContextMenuProvider>
          <QueryWidget
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
