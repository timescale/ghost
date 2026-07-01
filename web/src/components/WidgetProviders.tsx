import {
  ContextMenuContext,
  ContextMenuProvider,
  QueryWidgetProvider,
  Theme,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget-cdn';
import type { ReactNode } from 'react';

// WidgetProviders hosts the PopSQL query widget's shared contexts — the
// in-process results cache (the DuckDB-WASM worker), the widget theme/global
// config, and the right-click context-menu portal — at the app root, so they
// persist for the tab's lifetime. Keeping them above all layout/view switching
// means the results cache (and the in-memory query history that references its
// cached runs) is never torn down when panes mount or unmount.
export function WidgetProviders({ children }: { children: ReactNode }) {
  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme={Theme.light}>
        <ContextMenuProvider>
          {children}
          {/* Single context-menu render surface for every widget in the tree. */}
          <ContextMenuContext.Consumer>
            {({ render }: { render: () => ReactNode }) => render()}
          </ContextMenuContext.Consumer>
        </ContextMenuProvider>
      </QueryWidgetProvider>
    </TimescaleResultsCacheContextProvider>
  );
}
