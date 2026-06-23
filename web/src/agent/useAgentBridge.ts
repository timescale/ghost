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
    // The command currently being executed and its AbortController, so a
    // 'cancel' event targeting it can abort the command's own in-flight query.
    // Only one command runs at a time (the server serializes dispatch).
    let inFlightCommandId: string | null = null;
    let inFlightAbort: AbortController | null = null;
    // Request IDs whose 'cancel' arrived before the 'command' did. The server
    // can resolve a request (cancel + supersede) while the command-dispatch send
    // is still racing on its event channel, so the command can land *after* the
    // cancel. Remembering pre-empted IDs lets runCommand skip a command that was
    // already canceled, instead of running an abandoned query. Bounded: an entry
    // is removed as soon as its (late) command arrives.
    const preemptedCommandIds = new Set<string>();

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
      getLastRun: () => useAgentStore.getState().lastRun,
    };

    const runCommand = async (command: AgentCommand) => {
      if (!clientId) return;
      // A cancel for this command already arrived (it raced ahead of the command
      // on the event stream): the server has dropped the request, so don't run
      // an abandoned query. Just clear the pre-empted marker.
      if (preemptedCommandIds.delete(command.id)) return;
      const abort = new AbortController();
      inFlightCommandId = command.id;
      inFlightAbort = abort;
      const stopHeartbeat = startHeartbeat(clientId, command.id);
      try {
        const result = await dispatch(
          command.type,
          command.payload,
          deps,
          abort.signal,
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
        if (inFlightCommandId === command.id) {
          inFlightCommandId = null;
          inFlightAbort = null;
        }
      }
    };

    // cancelCommand aborts the in-flight command when the server signals the
    // request should be abandoned (caller canceled, timed out, or another tab
    // took over). Aborting the command's own AbortController cancels only its
    // query (the visualize handler wires the signal to its run) — never an
    // unrelated query the user kicked off. The aborted run completes as
    // 'canceled', which rejects the dispatcher's runQuery and lets runCommand
    // finish (its sendError is then a no-op since the server already dropped the
    // request). If the cancel races ahead of the command, remember it so
    // runCommand skips that command when it lands.
    const cancelCommand = (requestId: string) => {
      if (inFlightCommandId === requestId) {
        inFlightAbort?.abort();
        return;
      }
      preemptedCommandIds.add(requestId);
    };

    source.onopen = () => {
      // The stream is open: the backend is alive. (In plain `ghost serve` this
      // is the only signal; with an agent bridge a status event follows.)
      useAgentStore.getState().setConnected();
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
        useAgentStore.getState().setStatus(parsed.clientId, parsed.active);
      } else if (parsed.type === 'command') {
        void runCommand(parsed.command);
      } else if (parsed.type === 'cancel') {
        cancelCommand(parsed.requestId);
      }
    };

    source.onerror = () => {
      // EventSource auto-reconnects; reflect the dropped connection meanwhile.
      // Once it reconnects, onopen fires again and clears the disconnected
      // state. This is what powers the "backend disconnected" banner.
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
