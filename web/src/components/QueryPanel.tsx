import type React from 'react';
import {
  ContextMenuContext,
  ContextMenuProvider,
  ExecuteQueryEngine,
  QueryWidget,
  QueryWidgetProvider,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget';
import { useState } from 'react';

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

  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme="light">
        <ContextMenuProvider>
          <QueryWidget
            id={`ghost-${databaseId}`}
            query={query}
            onQueryChange={setQuery}
            sessionKey={`ghost-${databaseId}`}
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
