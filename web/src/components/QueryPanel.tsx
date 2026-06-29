import { useQueryClient } from '@tanstack/react-query';
import {
  ContextMenuContext,
  ContextMenuProvider,
  type ExecuteQueryData,
  ExecuteQueryEngine,
  type GetExecuteQueryDataArgs,
  type OnQueryCompleteArgs,
  QueryWidget,
  type QueryWidgetApiRef,
  QueryWidgetProvider,
  ResultsCacheContext,
  Theme,
  TimescaleResultsCacheContextProvider,
} from '@timescale/popsql-query-widget-cdn';
import type React from 'react';
import {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import type { QueryOutcome } from '../agent/executor';
import { type Executor, registerExecutor } from '../agent/executor';
import { fetchRunData, type ResultsCacheClient } from '../agent/runData';
import { useAgentStore } from '../agent/store';
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
  // Imperative handle to the widget, used by the agent executor to run queries.
  const apiRef = useRef<QueryWidgetApiRef>(null);
  // Resolvers for agent-initiated runs, keyed by runId; resolved in
  // handleQueryComplete when the matching run finishes.
  const pendingRuns = useRef<Map<string, (outcome: QueryOutcome) => void>>(
    new Map(),
  );
  const queryClient = useQueryClient();

  const setAgentLastRun = useAgentStore((s) => s.setLastRun);
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
      // The Postgres command-tag count (rows touched by a DML command, or rows
      // returned by a SELECT) is only carried on the success branch; it's zero
      // for a failed/canceled run.
      const rowsAffected = succeeded ? args.rowsAffected : 0;
      // Resolve any agent run awaiting this completion, and record the run as
      // the latest for the agent's uiState/chart tools.
      const failed = 'error' in args;
      if (succeeded || failed) {
        setAgentLastRun({
          databaseId,
          runId: args.runId,
          status: succeeded ? 'success' : 'failed',
          rowCount: args.rowCount ?? 0,
          rowsAffected,
          error: failed ? args.error : undefined,
        });
      }
      // Resolve the agent run on any terminal outcome, including cancellation
      // (neither succeeded nor failed) — otherwise an aborted run would leave
      // the dispatcher's runQuery promise pending forever (and its heartbeat
      // running). A canceled run is reported as failed with a clear message.
      const pending = pendingRuns.current.get(args.runId);
      if (pending) {
        pendingRuns.current.delete(args.runId);
        pending({
          runId: args.runId,
          status: succeeded ? 'success' : 'failed',
          // args.rowCount is the true total row count produced by the query,
          // independent of any cap applied when reading rows back for the agent.
          rowCount: args.rowCount ?? 0,
          rowsAffected,
          error: failed
            ? args.error
            : succeeded
              ? undefined
              : 'the query was canceled',
        });
      }
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
    [queryClient, databaseId, addQueryHistoryEntry, setAgentLastRun],
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

  // When this panel unmounts (or the database changes, which remounts it), any
  // agent run still awaiting handleQueryComplete will never settle — the
  // completion handler won't fire for a torn-down instance. Settle those
  // pending runs (resolving with status: 'failed') and abort the in-flight
  // query so the agent dispatcher's runQuery promise resolves (and its
  // heartbeat stops), instead of hanging the MCP tool call indefinitely.
  // biome-ignore lint/correctness/useExhaustiveDependencies: databaseId is the reset trigger (panel re-targets on DB change), not read in the body
  useEffect(() => {
    const pending = pendingRuns.current;
    return () => {
      apiRef.current?.cancelQuery();
      for (const [runId, resolve] of pending) {
        resolve({
          runId,
          status: 'failed',
          rowCount: 0,
          rowsAffected: 0,
          error: 'the database panel was torn down before the query completed',
        });
      }
      pending.clear();
    };
  }, [databaseId]);

  const runQuery = useCallback(
    (sql: string, signal: AbortSignal): Promise<QueryOutcome> => {
      onQueryChange(sql);
      // Use a plain UUID: the serve backend parses runId as a uuid.UUID, so a
      // prefixed value (e.g. `agent-<uuid>`) fails JSON decoding with "invalid
      // JSON body", surfacing in the UI as a generic "Something went wrong".
      const runId = crypto.randomUUID();
      return new Promise<QueryOutcome>((resolve, reject) => {
        // Already canceled before we could start (e.g. the MCP request was
        // abandoned during awaitExecutor): don't start a query at all.
        if (signal.aborted) {
          reject(new Error('the query was canceled'));
          return;
        }
        // Defer to the next tick so the editor reflects the new SQL before the
        // widget reads it for execution. The widget is guaranteed ready here:
        // ExecutorBridge only registers this executor once the results-cache
        // client has initialized (see below), and the agent dispatcher awaits
        // the executor before calling runQuery. So executeQuery returning falsy
        // here genuinely means a query is already in progress.
        setTimeout(() => {
          if (signal.aborted) {
            reject(new Error('the query was canceled'));
            return;
          }
          // The panel was torn down (e.g. a database switch remounted it)
          // between scheduling this tick and running it: there's no widget to
          // run against. Reject with the real cause rather than the misleading
          // "a query is already running" below. (The unmount-cleanup effect
          // also rejects already-pending runs; this covers the gap before a run
          // becomes pending.)
          const api = apiRef.current;
          if (!api) {
            reject(
              new Error(
                'the database panel was torn down before the query could start',
              ),
            );
            return;
          }
          const started = api.executeQuery(sql, runId);
          if (!started) {
            reject(new Error('a query is already running; try again shortly'));
            return;
          }
          pendingRuns.current.set(started, resolve);
          // Cancel only THIS run when the agent's command is abandoned. The
          // resulting 'canceled' completion resolves the pending run as failed
          // (see handleQueryComplete). Scoping cancellation to the started
          // runId — and only firing while the run is still pending — avoids
          // aborting an unrelated query the user kicked off in the meantime.
          signal.addEventListener(
            'abort',
            () => {
              if (pendingRuns.current.has(started))
                apiRef.current?.cancelQuery();
            },
            { once: true },
          );
        }, 0);
      });
    },
    [onQueryChange],
  );

  return (
    <TimescaleResultsCacheContextProvider baseUrl={window.location.origin}>
      <QueryWidgetProvider theme={Theme.light}>
        <ContextMenuProvider>
          <ExecutorBridge databaseId={databaseId} runQuery={runQuery} />
          <div className="flex flex-auto flex-col overflow-hidden">
            <QueryWidget
              apiRef={apiRef}
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

// ExecutorBridge registers an agent [Executor] for the currently-mounted
// database. It must render inside the widget's ResultsCacheContext so it can
// read cached run results. Rendering nothing, it only wires the imperative
// run/read capabilities into the module-level executor registry, so the
// app-level agent dispatcher can drive this database panel.
function ExecutorBridge({
  databaseId,
  runQuery,
}: {
  databaseId: string;
  runQuery: (sql: string, signal: AbortSignal) => Promise<QueryOutcome>;
}) {
  const { client } = useContext(ResultsCacheContext) as {
    client: ResultsCacheClient | null;
  };

  useEffect(() => {
    // Gate registration on the results-cache client being ready. This client is
    // the widget's readiness signal: it's initialized asynchronously, and the
    // widget can't start a run (executeQuery returns null) until it exists.
    // Registering only once it's ready means the agent dispatcher — which
    // awaits the executor before running — never races a half-initialized
    // widget, so runQuery can execute immediately without polling/retrying.
    if (!client) return;
    const executor: Executor = {
      databaseId,
      runQuery,
      getRunData: (runId, limit) => fetchRunData(client, runId, limit),
    };
    return registerExecutor(executor);
  }, [databaseId, runQuery, client]);

  return null;
}
