// Cadence at which the browser posts heartbeats to the server while a command
// is in flight. Must be comfortably under the server's 60s idle timeout so a
// long-running query doesn't trip it.
export const HEARTBEAT_INTERVAL_MS = 15_000;

interface RespondBody {
  clientId: string;
  requestId: string;
  type: 'heartbeat' | 'result' | 'error';
  data?: unknown;
  error?: string;
}

async function respond(body: RespondBody): Promise<void> {
  await fetch('/api/agent/respond', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).catch(() => {
    // A failed respond (e.g. the request already timed out server-side) is not
    // actionable here; the server will have cleaned up the pending request.
  });
}

export function sendResult(
  clientId: string,
  requestId: string,
  data: unknown,
): Promise<void> {
  return respond({ clientId, requestId, type: 'result', data });
}

export function sendError(
  clientId: string,
  requestId: string,
  message: string,
): Promise<void> {
  return respond({ clientId, requestId, type: 'error', error: message });
}

// startHeartbeat posts heartbeats on an interval until the returned stop
// function is called. Used to keep a long-running command alive on the server.
export function startHeartbeat(
  clientId: string,
  requestId: string,
): () => void {
  const timer = setInterval(() => {
    void respond({ clientId, requestId, type: 'heartbeat' });
  }, HEARTBEAT_INTERVAL_MS);
  return () => clearInterval(timer);
}

// activateClient posts to make this tab the active (agent-controlled) tab — the
// "take over" action.
export async function activateClient(clientId: string): Promise<void> {
  await fetch('/api/agent/activate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ clientId }),
  });
}
