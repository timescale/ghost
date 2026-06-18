import { CopyButton } from '../components/CopyButton';
import { serveCommand } from './serveCommand';
import { useAgentStore } from './store';

// DisconnectedBanner appears when the SSE stream to the backend drops after
// having connected — i.e. the `ghost serve` / `ghost mcp` process that served
// this page has gone away. It tells the user how to bring the backend back on
// the same port so this tab reconnects (EventSource retries automatically).
export function DisconnectedBanner() {
  const connectionState = useAgentStore((s) => s.connectionState);

  if (connectionState !== 'disconnected') return null;

  const command = serveCommand(window.location);

  return (
    <div className="flex items-center justify-center gap-3 border-b border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-900">
      <span className="flex items-center gap-2">
        <span className="h-2 w-2 shrink-0 rounded-full bg-amber-500" />
        <span>
          <strong className="font-semibold">Disconnected from backend.</strong>{' '}
          The server is no longer running. Restart it to reconnect:
        </span>
      </span>
      <code className="rounded bg-amber-100 px-2 py-1 font-mono text-xs text-amber-950">
        {command}
      </code>
      <CopyButton text={command} />
    </div>
  );
}
