import { useAgentStore } from './store';
import { activateClient } from './transport';

// AgentStatusBanner shows whether this tab is the one an AI agent is currently
// driving. The active tab gets a subtle "controlling" indicator; an inactive
// (but connected) tab offers a "Take over" button to claim control — matching
// the incumbent-stays policy where opening/reloading a tab does not steal
// control automatically.
export function AgentStatusBanner() {
  const agentPresent = useAgentStore((s) => s.agentPresent);
  const connectionState = useAgentStore((s) => s.connectionState);
  const active = useAgentStore((s) => s.active);
  const clientId = useAgentStore((s) => s.clientId);

  // Only relevant when an agent bridge is present and currently connected.
  if (!agentPresent || connectionState !== 'connected') return null;

  if (active) {
    return (
      <span
        className="flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700"
        title="An AI agent can control this tab"
      >
        <span className="h-2 w-2 rounded-full bg-emerald-500" />
        Agent controlling this tab
      </span>
    );
  }

  return (
    <span className="flex items-center gap-2 rounded-full bg-amber-50 px-2.5 py-1 text-xs font-medium text-amber-700">
      <span className="h-2 w-2 rounded-full bg-amber-400" />
      Agent active in another tab
      <button
        type="button"
        onClick={() => clientId && activateClient(clientId)}
        disabled={!clientId}
        className="rounded bg-amber-600 px-1.5 py-0.5 text-white transition-colors hover:bg-amber-700 disabled:opacity-50"
      >
        Take over
      </button>
    </span>
  );
}
