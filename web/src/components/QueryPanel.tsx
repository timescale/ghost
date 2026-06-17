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
import { useCallback, useMemo, useRef, useState } from 'react';

import { useAutocompletePlugin } from '../autocomplete/useAutocompletePlugin';
import { useServeStore } from '../store';
import { ChartArea } from './chart/ChartArea';
import { ResultViewToggle } from './chart/ResultViewToggle';
import { Icon } from './Icon';
import { QueryHistoryModal } from './QueryHistoryModal';

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
  // The runId of the most recent successful run, used to load results for the
  // chart. Persists across view toggles; updated on each successful run.
  const [chartRunId, setChartRunId] = useState<string | null>(null);
  const [historyOpen, setHistoryOpen] = useState(false);
  // True only while the user is actively dragging the editor resize handle. In
  // 'split' mode the widget also fires onResizeEditor programmatically on any
  // container reflow (e.g. when toggling between table and chart views), which
  // clamps the editor height to the current container size. Persisting those
  // programmatic values shrinks the editor on view switches, so we only persist
  // height changes that occur during a real user drag.
  const isResizingEditor = useRef(false);
  // Maps a run's id to the exact SQL that was executed (a selection, if active,
  // otherwise the full editor contents), captured in getExecuteQueryData so the
  // history entry recorded on completion reflects what actually ran.
  const runSqlById = useRef<Map<string, string>>(new Map());
  const queryClient = useQueryClient();

  const addQueryHistoryEntry = useServeStore((s) => s.addQueryHistoryEntry);
  const appendEditorSql = useServeStore((s) => s.appendEditorSql);

  const resultView = useServeStore((s) => s.resultView);
  const setResultView = useServeStore((s) => s.setResultView);
  const chartConfig = useServeStore((s) => s.chartConfig);
  const setChartConfig = useServeStore((s) => s.setChartConfig);
  const showTable = resultView === 'table';

  const autocompletePlugin = useAutocompletePlugin(databaseId);
  const editorPlugins = useMemo(
    () => [autocompletePlugin],
    [autocompletePlugin],
  );

  const handleQueryComplete = useCallback(
    (args: OnQueryCompleteArgs) => {
      setStatementCount(args.statementCount ?? 0);
      // 'rowsAffected' is only present on the success branch of the union, so
      // this narrows to a successful run; track its id for charting.
      const succeeded = 'rowsAffected' in args;
      if (succeeded) setChartRunId(args.runId);
      // Record every completed run (success or failure) in the history; skip
      // cancellations, which have no real outcome. Success and failure are the
      // two branches carrying 'rowsAffected'/'error'; the canceled branch has
      // neither. The SQL was stashed by getExecuteQueryData under this runId.
      const sql = runSqlById.current.get(args.runId);
      runSqlById.current.delete(args.runId);
      if (sql !== undefined && (succeeded || 'error' in args)) {
        addQueryHistoryEntry(sql, succeeded);
      }
      // Refresh the schema (shared by the tree and autocomplete) after a
      // statement that may have altered it, so new objects appear without a
      // manual refresh.
      if (isSchemaChangingCommand(args.command)) {
        void queryClient.invalidateQueries({
          queryKey: ['schema', databaseId],
        });
      }
    },
    [queryClient, databaseId, addQueryHistoryEntry],
  );

  const renderToolbarAppendLeft = useCallback(
    ({ isRunning }: { isRunning: boolean }) => (
      // The history button and view toggle sit just right of the run button; the
      // statement count (multi-statement runs only) trails them.
      <div className="flex-auto flex items-center gap-2">
        <button
          type="button"
          onClick={() => setHistoryOpen(true)}
          aria-label="Query history"
          title="Query history"
          className="rounded border border-slate-300 bg-white p-1 text-slate-600 transition-colors hover:bg-slate-100 hover:text-slate-800"
        >
          <Icon name="history" size="sm" color="current" />
        </button>
        <ResultViewToggle value={resultView} onChange={setResultView} />
        {!isRunning && statementCount > 1 ? (
          <span className="text-xs text-slate-500">
            Executed {statementCount} statements
          </span>
        ) : null}
      </div>
    ),
    [resultView, setResultView, statementCount],
  );

  const getExecuteQueryData = useCallback(
    ({ runId, query: q }: GetExecuteQueryDataArgs): ExecuteQueryData => {
      // Stash the exact SQL being run so handleQueryComplete can record it.
      runSqlById.current.set(runId, q);
      return {
        engine: ExecuteQueryEngine.timescaleQuery,
        params: {
          projectId,
          serviceId: databaseId,
          query: q,
          runId,
        },
      };
    },
    [projectId, databaseId],
  );

  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme={Theme.light}>
        <ContextMenuProvider>
          <div className="flex flex-auto flex-col overflow-hidden">
            <QueryWidget
              // In table view the widget fills the pane (split layout); in
              // chart/editor view it shrinks to the editor and we render the
              // chart area below it.
              className={showTable ? 'flex-auto' : undefined}
              resizeHandles={showTable ? 'split' : 'editor'}
              editorMinHeight={200}
              editorHeight={editorHeight}
              onResizeStart={() => {
                isResizingEditor.current = true;
              }}
              onResizeStop={() => {
                isResizingEditor.current = false;
              }}
              onResizeEditor={(height) => {
                if (isResizingEditor.current) onResizeEditor(height);
              }}
              id={databaseId}
              query={query}
              onQueryChange={onQueryChange}
              sessionKey={databaseId}
              runSelection
              runButtonLabelWithSelection="Run selection"
              hideResults={!showTable}
              onQueryComplete={handleQueryComplete}
              renderToolbarAppendLeft={renderToolbarAppendLeft}
              getExecuteQueryData={getExecuteQueryData}
              editorPlugins={editorPlugins}
            />
            {showTable ? null : (
              <ChartArea
                runId={chartRunId}
                view={resultView}
                config={chartConfig}
                onConfigChange={setChartConfig}
              />
            )}
          </div>
          <ContextMenuContext.Consumer>
            {({ render }: { render: () => React.ReactNode }) => render()}
          </ContextMenuContext.Consumer>
          {historyOpen ? (
            <QueryHistoryModal
              onClose={() => setHistoryOpen(false)}
              onApply={(sql) => {
                onQueryChange(sql);
                setHistoryOpen(false);
              }}
              onAppend={(sql) => {
                appendEditorSql(sql);
                setHistoryOpen(false);
              }}
            />
          ) : null}
        </ContextMenuProvider>
      </QueryWidgetProvider>
    </TimescaleResultsCacheContextProvider>
  );
}
