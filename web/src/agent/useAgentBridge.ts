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
    // The request ID of a 'cancel' that arrived before its 'command' did.
    // Defensive: the server (see internal/serve/agent.go) now sends a cancel
    // only from Request, and only AFTER the command has been enqueued to this
    // same client's FIFO event channel — so a cancel can't normally overtake
    // its command, and cancelCommand should hit the inFlightCommandId branch.
    // This slot guards the residual case of out-of-order delivery (e.g. an
    // EventSource/proxy reordering quirk): if a cancel still lands first,
    // runCommand skips the abandoned command when it arrives. A single slot
    // suffices because the server keeps at most one command in flight; it's
    // overwritten by any later cancel and cleared when its command arrives, so
    // it can't grow unbounded.
    let preemptedCommandId: string | null = null;

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
      if (preemptedCommandId === command.id) {
        preemptedCommandId = null;
        return;
      }
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
      preemptedCommandId = requestId;
    };

    // abortInFlightCommand aborts whatever command is currently running. Used
    // when the SSE stream drops (or the bridge tears down): the server treats
    // that disconnect as ErrClientDisconnected and ends the request, so no
    // later cancel can be delivered. Aborting here cancels a long visualize
    // query (its run rejects, and runCommand's finally stops the heartbeat) so
    // it doesn't keep running — and can't still be mutating the UI when
    // EventSource reconnects with a fresh client and a new command arrives.
    const abortInFlightCommand = () => {
      inFlightAbort?.abort();
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
      // state. This is what powers the "backend disconnected" banner. Abort any
      // in-flight command: the server has already ended its request on the
      // disconnect, so finishing it would be wasted work that could still be
      // mutating the UI when the stream reconnects with a new command.
      abortInFlightCommand();
      useAgentStore.getState().setDisconnected();
    };

    return () => {
      abortInFlightCommand();
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
