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
import {
  type ReactNode,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';

import { CopyButton } from './CopyButton';

interface Props {
  query: string;
  // Monaco language for the read-only editor. Defaults to the widget's SQL
  // language; pass 'plaintext' for prose content (e.g. object comments) that
  // shouldn't be SQL-highlighted.
  language?: string;
  // Extra actions rendered in the toolbar to the left of the copy button.
  toolbarActions?: ReactNode;
  // When true, the widget flexes to fill its parent container's height instead
  // of auto-sizing to the SQL. The parent must be a flex column with a
  // constrained height. Keeps the toolbar pinned to the bottom.
  fill?: boolean;
}

// SqlCodeView renders read-only, syntax-highlighted SQL using the PopSQL
// QueryWidget with the run button, results, status, and search all hidden — so
// only the code editor and a copy button (in the toolbar) are visible. Used to
// display object definitions (indexes, functions, procedures) with the same
// highlighting as the main query editor.
export function SqlCodeView({ query, language, toolbarActions, fill }: Props) {
  // When `fill` is set, the widget can't flex on its own (its editor auto-sizes
  // to the SQL), so we measure the wrapper and feed the height back in as a
  // controlled editorHeight. The widget subtracts its own toolbar height, so
  // passing the full container height keeps the toolbar pinned to the bottom.
  const wrapperRef = useRef<HTMLDivElement | null>(null);
  const [fillHeight, setFillHeight] = useState<number | undefined>(undefined);
  useEffect(() => {
    const el = wrapperRef.current;
    if (!fill || !el) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setFillHeight(entry.contentRect.height);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [fill]);

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
    () => (
      <div className="flex items-center gap-2">
        <CopyButton text={query} />
        {toolbarActions}
      </div>
    ),
    [query, toolbarActions],
  );

  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme={Theme.light}>
        <ContextMenuProvider>
          <div
            ref={wrapperRef}
            className={fill ? 'flex min-h-0 flex-auto' : undefined}
          >
            <QueryWidget
              id="definition-viewer"
              className={fill ? 'flex-auto' : undefined}
              query={query}
              getExecuteQueryData={getExecuteQueryData}
              readonlyEditor
              disableRun
              hideRunButton
              hideResults
              hideSessionStatus
              hideSearchInput
              resizeHandles="none"
              editorHeight={fill ? fillHeight : undefined}
              renderToolbarAppendRight={renderToolbarAppendRight}
              editorLanguage={language}
              editorOptions={{
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                // Prose wraps to the modal width; SQL keeps Monaco's default
                // no-wrap + horizontal scroll.
                ...(language === 'plaintext'
                  ? { wordWrap: 'on' as const }
                  : {}),
              }}
            />
          </div>
          <ContextMenuContext.Consumer>
            {({ render }: { render: () => React.ReactNode }) => render()}
          </ContextMenuContext.Consumer>
        </ContextMenuProvider>
      </QueryWidgetProvider>
    </TimescaleResultsCacheContextProvider>
  );
}
