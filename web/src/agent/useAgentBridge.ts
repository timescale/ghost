import { useEffect } from 'react';

import type { ResultView } from '../components/chart/types';
import { useServeStore } from '../store';
import { type DispatchDeps, dispatch } from './dispatch';
import { useAgentStore } from './store';
import { sendError, sendResult, startHeartbeat } from './transport';
import type { AgentCommand, AgentServerEvent } from './types';

interface Database {
  id: string;
  name: string;
}

// useAgentBridge opens the agent SSE stream and processes commands dispatched
// by the MCP server. It lives at the app level (outside the per-database
// QueryPanel) so the connection survives database switches. `databases` is the
// loaded database list, used to resolve a name-or-id ref to an id.
//
// Only the active tab executes commands; status events from the server tell
// each tab whether it is active. Commands are always for the active tab (the
// server only dispatches to it).
export function useAgentBridge(databases: Database[]): void {
  // Keep a ref-free snapshot accessor by reading the stores' getState() inside
  // the handler, so the EventSource effect doesn't need to re-subscribe on
  // every state change.
  useEffect(() => {
    const source = new EventSource('/api/agent/events');
    let clientId: string | null = null;

    const resolveDatabaseId = (ref: string): string | null => {
      const list = databasesRef.current;
      const byId = list.find((d) => d.id === ref);
      if (byId) return byId.id;
      const byName = list.find((d) => d.name === ref);
      return byName ? byName.id : null;
    };

    const deps: DispatchDeps = {
      resolveDatabaseId,
      getState: () => {
        const s = useServeStore.getState();
        return {
          selectedDatabaseId: s.selectedDatabaseId,
          editorSql: s.editorSql,
          chartConfig: s.chartConfig,
          resultView: s.resultView,
        };
      },
      setSelectedDatabaseId: (id) =>
        useServeStore.getState().setSelectedDatabaseId(id),
      setEditorSql: (sql) => useServeStore.getState().setEditorSql(sql),
      setResultView: (view: ResultView) =>
        useServeStore.getState().setResultView(view),
      setChartConfig: (config) =>
        useServeStore.getState().setChartConfig(config),
      getLastRunId: () => useAgentStore.getState().lastRun?.runId ?? null,
    };

    const getLastRun = () => {
      const run = useAgentStore.getState().lastRun;
      return run
        ? {
            runId: run.runId,
            status: run.status,
            rowCount: run.rowCount,
            error: run.error,
          }
        : null;
    };

    const runCommand = async (command: AgentCommand) => {
      if (!clientId) return;
      const stopHeartbeat = startHeartbeat(clientId, command.id);
      try {
        const result = await dispatch(
          command.type,
          command.payload,
          deps,
          getLastRun,
        );
        await sendResult(clientId, command.id, result);
      } catch (err) {
        await sendError(
          clientId,
          command.id,
          err instanceof Error ? err.message : String(err),
        );
      } finally {
        stopHeartbeat();
      }
    };

    source.onmessage = (event) => {
      let parsed: AgentServerEvent;
      try {
        parsed = JSON.parse(event.data) as AgentServerEvent;
      } catch {
        return;
      }
      if (parsed.type === 'status') {
        clientId = parsed.clientId;
        useAgentStore.getState().setConnection(parsed.clientId, parsed.active);
      } else if (parsed.type === 'command') {
        void runCommand(parsed.command);
      }
    };

    source.onerror = () => {
      // EventSource auto-reconnects; reflect the dropped connection meanwhile.
      useAgentStore.getState().setDisconnected();
    };

    return () => {
      source.close();
      useAgentStore.getState().setDisconnected();
    };
    // The handler reads the latest databases via databasesRef, so this effect
    // intentionally runs once (connection lifecycle), not per databases change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Keep the latest database list available to the (stable) SSE handler.
  databasesRef.current = databases;
}

// Module-level ref so the long-lived EventSource handler always resolves refs
// against the freshest database list without re-subscribing.
const databasesRef: { current: Database[] } = { current: [] };
