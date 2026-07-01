import { useQueryClient } from '@tanstack/react-query';
import {
  type ExecuteQueryData,
  ExecuteQueryEngine,
  type GetExecuteQueryDataArgs,
  type OnQueryCompleteArgs,
  QueryWidget,
  type QueryWidgetApiRef,
} from '@timescale/popsql-query-widget-cdn';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { QueryOutcome } from '../agent/executor';
import { type Executor, registerExecutor } from '../agent/executor';
import { evictRuns, fetchRunData } from '../agent/runData';
import { useAgentStore } from '../agent/store';
import { useResultsCacheClient } from '../agent/useResultsCacheClient';
import { useAutocompletePlugin } from '../autocomplete/useAutocompletePlugin';
import { type QueryHistoryEntry, useServeStore } from '../store';
import { ChartArea } from './chart/ChartArea';
import { ResultViewToggle } from './chart/ResultViewToggle';
import type { ResultView } from './chart/types';
import { useChartConfigRecorder } from './chart/useChartConfigRecorder';
import { useChartData } from './chart/useChartData';
import { HistoryModal, type HistoryTab } from './HistoryModal';
import { Icon } from './Icon';
import { useEditorHistoryRecorder } from './useEditorHistoryRecorder';
import { WIDGET_REFERENCE_ID } from './widgetReference';

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
// database, plus the chart area and the unified history modal. The sessionKey
// is derived from the database ID so switching databases automatically
// invalidates the session (and tears down the in-process PG connection on the
// Go side). The widget's shared providers (results cache, theme, context menu)
// live at the app root (see WidgetProviders), so this panel can mount/unmount
// with layout changes without tearing down the cached run results.
export function QueryPanel({
  projectId,
  databaseId,
  databaseName,
  query,
  onQueryChange,
  editorHeight,
  onResizeEditor,
}: Props) {
  const [statementCount, setStatementCount] = useState(0);
  // The runId of the most recent successful run, used to load results for the
  // chart. Persists across view toggles; updated on each successful run.
  const [chartRunId, setChartRunId] = useState<string | null>(null);
  // The run currently displayed in the widget's results grid. Controlled so we
  // can point the grid at a historical run ("Open in editor" from query history)
  // without re-executing it; kept in sync with the widget's own runs via
  // onQueryRun.
  const [activeRunId, setActiveRunId] = useState<string | null>(null);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyTab, setHistoryTab] = useState<HistoryTab>('editor');
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

  // The widget's in-process results cache. Used to read cached run results and
  // to evict the oldest run when a new one pushes past the retention limit.
  const client = useResultsCacheClient();
  const clientRef = useRef(client);
  clientRef.current = client;

  const setAgentLastRun = useAgentStore((s) => s.setLastRun);
  const addQueryHistoryEntry = useServeStore((s) => s.addQueryHistoryEntry);
  const appendEditorSql = useServeStore((s) => s.appendEditorSql);
  const setSelectedDatabaseId = useServeStore((s) => s.setSelectedDatabaseId);

  // Record the full editor contents into editor history (debounced) whenever
  // the content is freshly authored — the user typing, or the agent authoring
  // SQL via MCP. Runs themselves are captured separately by query history.
  // markApplied is called only before edits that replay content from a history
  // panel (Open in editor, Replace/Append) so those aren't re-recorded as fresh
  // drafts (which would just churn the ordering of an entry already in history).
  const { markApplied: markEditorApplied } = useEditorHistoryRecorder(query);

  const resultView = useServeStore((s) => s.resultView);
  const setResultView = useServeStore((s) => s.setResultView);
  const chartConfig = useServeStore((s) => s.chartConfig);
  const setChartConfig = useServeStore((s) => s.setChartConfig);
  const chartEditorWidth = useServeStore((s) => s.chartEditorWidth);
  const setChartEditorWidth = useServeStore((s) => s.setChartEditorWidth);
  const showTable = resultView === 'table';

  // Read the cached results for the most recent successful run, shared by the
  // chart area and the chart-config history tab's live preview.
  const {
    data: chartData,
    loading: chartLoading,
    error: chartError,
  } = useChartData(chartRunId);
  // Records chart configs into history as they render — the user's or the
  // agent's (via MCP). markApplied prevents a config replayed from a history
  // panel from being re-recorded as a fresh edit.
  const { recordRenderSuccess, markApplied } =
    useChartConfigRecorder(chartConfig);

  const autocompletePlugin = useAutocompletePlugin(databaseId);
  const editorPlugins = useMemo(
    () => [autocompletePlugin],
    [autocompletePlugin],
  );

  const openHistory = useCallback((tab: HistoryTab) => {
    setHistoryTab(tab);
    setHistoryOpen(true);
  }, []);

  // Keep our controlled run id in sync with the widget's own runs: each run the
  // widget starts becomes the active run shown in the grid.
  const handleQueryRun = useCallback(({ runId }: { runId: string }) => {
    setActiveRunId(runId);
  }, []);

  const handleQueryComplete = useCallback(
    (args: OnQueryCompleteArgs) => {
      setStatementCount(args.statementCount ?? 0);
      // 'rowsAffected' is only present on the success branch of the union, so
      // this narrows to a successful run; track its id for charting.
      const succeeded = 'rowsAffected' in args;
      // Point the chart at this run if it succeeded; otherwise clear it so a
      // failed or canceled run doesn't leave the previous successful run's
      // data on screen under the new (unrelated) SQL when the chart view is
      // shown.
      setChartRunId(succeeded ? args.runId : null);
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
      // Record every completed run in the history, including canceled ones: a
      // canceled run can still have produced (partial) results that the widget
      // can display, so we keep it in history and retain its cache entry (not
      // deleted while it remains in history) so the widget can still display
      // whatever it produced. Its cache entry is only evicted along with the
      // entry itself — by retention eviction (below) or a manual delete/clear
      // in the query-history panel. The SQL was stashed by getExecuteQueryData
      // under this runId.
      const sql = runSqlById.current.get(args.runId);
      runSqlById.current.delete(args.runId);
      if (sql) {
        // Record the distinct run (with its cached results) and evict the
        // oldest run's results once we exceed the retention limit. The
        // just-completed run is the newest entry, so it's never the one
        // evicted.
        const evicted = addQueryHistoryEntry({
          runId: args.runId,
          databaseId,
          databaseName,
          sql,
          // Capture the chart config as of run completion. Read live from the
          // store (not a render-time closure) so an agent's ghost_visualize —
          // which applies its config before running — is recorded with that
          // config, not a stale one.
          chartConfig: useServeStore.getState().chartConfig,
          ts: Date.now(),
          status: succeeded ? 'success' : failed ? 'failed' : 'canceled',
          rowCount: args.rowCount ?? 0,
        });
        evictRuns(clientRef.current, evicted);
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
    [
      queryClient,
      databaseId,
      databaseName,
      addQueryHistoryEntry,
      setAgentLastRun,
    ],
  );

  const renderToolbarAppendLeft = useCallback(
    ({ isRunning }: { isRunning: boolean }) => (
      // The history button and view toggle sit just right of the run button; the
      // statement count (multi-statement runs only) trails them.
      <div className="flex-auto flex items-center gap-2">
        <button
          type="button"
          onClick={() => openHistory('query')}
          aria-label="History"
          title="History"
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
    [resultView, setResultView, statementCount, openHistory],
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
      // Push the agent's SQL into the editor. Deliberately NOT marked as
      // applied: agent-authored SQL is fresh content (like the user typing it),
      // so it should flow through the recorder into editor history.
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

  // Register an agent Executor for the currently-mounted database, gated on the
  // results-cache client being ready (its initialization is the widget's
  // readiness signal). Registering only once it's ready means the agent
  // dispatcher — which awaits the executor before running — never races a
  // half-initialized widget.
  useEffect(() => {
    if (!client) return;
    const executor: Executor = {
      databaseId,
      runQuery,
      getRunData: (runId, limit) => fetchRunData(client, runId, limit),
    };
    return registerExecutor(executor);
  }, [databaseId, runQuery, client]);

  // Drop main-view references to runs whose cached results were just evicted
  // in the query-history panel (delete/clear). The results grid (activeRunId)
  // and chart (chartRunId) point into the same cache; if either still targets
  // an evicted run, its lazy reads would hit a missing cache entry and surface
  // an error under SQL that looks like it just ran fine. Clear them so the
  // grid/chart fall back to their empty state instead.
  const handleRunsEvicted = useCallback((runIds: string[]) => {
    const evicted = new Set(runIds);
    setActiveRunId((id) => (id !== null && evicted.has(id) ? null : id));
    setChartRunId((id) => (id !== null && evicted.has(id) ? null : id));
  }, []);

  // Apply a config picked from chart-config history: mark it so the recorder
  // doesn't treat it as a fresh user edit, push it into the editor, and close.
  const handleApplyConfig = useCallback(
    (next: string) => {
      markApplied(next);
      setChartConfig(next);
      setHistoryOpen(false);
    },
    [markApplied, setChartConfig],
  );

  // Make a run picked from query history the active run in the main view: load
  // its SQL into the editor, restore the chart config and result view the user
  // chose while previewing it, point the results grid at its cached rows, and
  // (if it succeeded) feed the chart from it. Then close the modal.
  const handleOpenRun = useCallback(
    (entry: QueryHistoryEntry, view: ResultView, config: string) => {
      // Query history is global across databases, so a run may have executed
      // against a different database than the one currently selected. Switch
      // the selection to the run's database so pressing Run re-executes its
      // SQL against the database it actually ran against (not the current one,
      // which would be a real footgun for DDL/DML). The panel isn't keyed by
      // database, so this only re-renders it with the new databaseId (no
      // remount) — the activeRunId/chartRunId set below persist and keep
      // pointing at this run's cached results.
      if (entry.databaseId !== databaseId) {
        setSelectedDatabaseId(entry.databaseId);
      }
      markEditorApplied(entry.sql);
      onQueryChange(entry.sql);
      markApplied(config);
      setChartConfig(config);
      setResultView(view);
      setActiveRunId(entry.runId);
      // Feed the chart from this run only if it fully succeeded; a failed or
      // canceled run has no complete chartable results, so clear any prior
      // chart source rather than leaving an unrelated run's data on screen.
      setChartRunId(entry.status === 'success' ? entry.runId : null);
      setHistoryOpen(false);
    },
    [
      databaseId,
      setSelectedDatabaseId,
      onQueryChange,
      markApplied,
      markEditorApplied,
      setChartConfig,
      setResultView,
    ],
  );

  return (
    <>
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
          // Keep every run's results cached (disable the widget's
          // keep-only-current-run eviction); ghost enforces the limit itself.
          referenceId={WIDGET_REFERENCE_ID}
          // Controlled run id: lets us display a historical run in the grid.
          // Synced with the widget's own runs via onQueryRun.
          runId={activeRunId}
          onQueryRun={handleQueryRun}
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
            view={resultView}
            config={chartConfig}
            onConfigChange={setChartConfig}
            data={chartData}
            loading={chartLoading}
            error={chartError}
            onRenderSuccess={recordRenderSuccess}
            editorWidth={chartEditorWidth}
            onEditorWidthChange={setChartEditorWidth}
          />
        )}
      </div>
      {historyOpen ? (
        <HistoryModal
          initialTab={historyTab}
          onClose={() => setHistoryOpen(false)}
          onApplyEditor={(sql) => {
            markEditorApplied(sql);
            onQueryChange(sql);
            setHistoryOpen(false);
          }}
          onAppendEditor={(sql) => {
            // appendEditorSql returns the combined contents; mark them applied
            // so the programmatic change isn't recorded as a fresh user draft.
            // Marking the store's own result (rather than recomputing the join)
            // keeps the marked baseline exactly in sync with what was written.
            markEditorApplied(appendEditorSql(sql));
            setHistoryOpen(false);
          }}
          onOpenRun={handleOpenRun}
          onRunsEvicted={handleRunsEvicted}
          onApplyConfig={handleApplyConfig}
          chartData={chartData}
          chartLoading={chartLoading}
          chartError={chartError}
        />
      ) : null}
    </>
  );
}
