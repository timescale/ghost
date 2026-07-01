// formatRelativeTime renders a compact, human-readable "time ago" string for
// the given epoch-millisecond timestamp (e.g. "just now", "5m ago", "3h ago",
// "2d ago"). Used for editor history timestamps, with the absolute time shown in
// a tooltip via formatAbsoluteTime.
export function formatRelativeTime(
  ts: number,
  now: number = Date.now(),
): string {
  const seconds = Math.max(0, Math.round((now - ts) / 1000));
  if (seconds < 45) return 'just now';
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  if (days < 7) return `${days}d ago`;
  const weeks = Math.round(days / 7);
  if (weeks < 5) return `${weeks}w ago`;
  const months = Math.round(days / 30);
  if (months < 12) return `${months}mo ago`;
  const years = Math.round(days / 365);
  return `${years}y ago`;
}

// formatAbsoluteTime renders the full local date and time for the given
// epoch-millisecond timestamp, used as the tooltip behind a relative time.
export function formatAbsoluteTime(ts: number): string {
  return new Date(ts).toLocaleString();
}
