import { create } from 'zustand';

// Status of the most recent query run executed in this tab (whether triggered
// by the user or the agent), surfaced to the agent via the uiState command.
export interface AgentLastRun {
  // The database this run executed against. The agent's chart/uiState tools use
  // it to ignore a run that belongs to a different database than the one whose
  // panel is currently mounted (e.g. after switching databases), so they never
  // read or chart results from the wrong panel.
  databaseId: string;
  runId: string;
  status: 'success' | 'failed';
  rowCount: number;
  // Postgres command-tag count for the run (rows touched by a DML command, or
  // rows returned by a SELECT). Zero for a failed run.
  rowsAffected: number;
  error?: string;
}

// Lifecycle of this tab's SSE connection to the backend. The stream doubles as
// a backend-liveness signal (served even in plain `ghost serve`):
//   'connecting'   – before the stream first opens (initial page load)
//   'connected'    – stream is open; the backend is alive
//   'disconnected' – the stream dropped after having been connected; the
//                    backend likely went away. EventSource auto-reconnects, so
//                    this clears back to 'connected' once the backend returns.
export type ConnectionState = 'connecting' | 'connected' | 'disconnected';

interface AgentStore {
  // The server-assigned ID for this tab's SSE connection, or null before the
  // stream connects (or when no agent bridge is present).
  clientId: string | null;
  // Whether this tab is the active (agent-controlled) tab. Multiple tabs may be
  // connected; exactly one is active.
  active: boolean;
  // Whether an agent bridge is present and has reported this tab's status.
  // False in plain `ghost serve` (no MCP), where the stream is liveness-only.
  agentPresent: boolean;
  // Lifecycle of the backend connection (see ConnectionState).
  connectionState: ConnectionState;
  // The most recent query run in this tab, or null if none yet.
  lastRun: AgentLastRun | null;
  setConnected: () => void;
  setStatus: (clientId: string, active: boolean) => void;
  setDisconnected: () => void;
  setLastRun: (run: AgentLastRun | null) => void;
}

export const useAgentStore = create<AgentStore>((set) => ({
  clientId: null,
  active: false,
  agentPresent: false,
  connectionState: 'connecting',
  lastRun: null,
  // The SSE stream opened: the backend is alive. Clear any stale agent state
  // and let the next status event (setStatus) restore it. This matters when a
  // tab that was bridge-backed reconnects to a plain `ghost serve` (no MCP):
  // the liveness stream reopens but no status event ever follows, so leaving
  // agentPresent/clientId set would wrongly keep showing "agent active in
  // another tab" with a stale clientId. A bridge-backed reconnect re-sends a
  // status event immediately, so the cleared state is transient there.
  setConnected: () =>
    set({
      clientId: null,
      active: false,
      agentPresent: false,
      connectionState: 'connected',
    }),
  setStatus: (clientId, active) =>
    set({ clientId, active, agentPresent: true, connectionState: 'connected' }),
  // The stream dropped. Clear the agent state since it's no longer valid; a
  // reconnect (onopen → setConnected) and any following status event restore it.
  setDisconnected: () =>
    set({
      clientId: null,
      active: false,
      agentPresent: false,
      connectionState: 'disconnected',
    }),
  setLastRun: (lastRun) => set({ lastRun }),
}));
