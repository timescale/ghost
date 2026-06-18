import { create } from 'zustand';

// Status of the most recent query run executed in this tab (whether triggered
// by the user or the agent), surfaced to the agent via the uiState command.
export interface AgentLastRun {
  runId: string;
  status: 'success' | 'failed';
  rowCount: number;
  error?: string;
}

interface AgentStore {
  // The server-assigned ID for this tab's SSE connection, or null before the
  // stream connects.
  clientId: string | null;
  // Whether this tab is the active (agent-controlled) tab. Multiple tabs may be
  // connected; exactly one is active.
  active: boolean;
  // Whether any tab is connected to the agent stream at all. Used to decide
  // whether to show the "another tab is active" affordance.
  connected: boolean;
  // The most recent query run in this tab, or null if none yet.
  lastRun: AgentLastRun | null;
  setConnection: (clientId: string | null, active: boolean) => void;
  setDisconnected: () => void;
  setLastRun: (run: AgentLastRun | null) => void;
}

export const useAgentStore = create<AgentStore>((set) => ({
  clientId: null,
  active: false,
  connected: false,
  lastRun: null,
  setConnection: (clientId, active) =>
    set({ clientId, active, connected: true }),
  setDisconnected: () =>
    set({ clientId: null, active: false, connected: false }),
  setLastRun: (lastRun) => set({ lastRun }),
}));
