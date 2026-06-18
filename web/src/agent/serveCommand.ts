// serveCommand builds the `ghost serve` command that restarts the backend on
// the same address as the current page, so reconnecting revives this exact
// frontend (the EventSource auto-reconnects once the server is back). The port
// is always pinned to the current one; --host is included only when the page
// isn't served from the default loopback address. -n (--no-open) is passed
// because this tab is already open and will reconnect on its own — no need to
// spawn another browser window.
export function serveCommand(location: {
  hostname: string;
  port: string;
}): string {
  const parts = ['ghost serve', '-n'];
  // Bare loopback hosts are the default; anything else (a LAN IP, a real
  // hostname) needs an explicit --host to rebind the same interface.
  const isLoopback =
    location.hostname === '127.0.0.1' ||
    location.hostname === 'localhost' ||
    location.hostname === '::1' ||
    location.hostname === '[::1]';
  if (!isLoopback) {
    parts.push(`--host ${location.hostname}`);
  }
  // The port is normally present; fall back to the scheme default if a proxy
  // stripped it (80 for http) so the command is still valid.
  const port = location.port || '80';
  parts.push(`-p ${port}`);
  return parts.join(' ');
}
