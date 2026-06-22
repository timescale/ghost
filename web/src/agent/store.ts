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
  // The SSE stream opened: the backend is alive. Agent presence is signaled
  // separately by the first status event (setStatus).
  setConnected: () => set({ connectionState: 'connected' }),
  setStatus: (clientId, active) =>
    set({ clientId, active, agentPresent: true, connectionState: 'connected' }),
  // The stream dropped. Keep agentPresent as-is so a reconnect restores prior
  // behavior; clear the active/clientId since they're no longer valid.
  setDisconnected: () =>
    set({ clientId: null, active: false, connectionState: 'disconnected' }),
  setLastRun: (lastRun) => set({ lastRun }),
}));
