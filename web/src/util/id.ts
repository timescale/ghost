// newId returns a fresh, unique identifier for a client-side entity (e.g. a
// history entry). Uses crypto.randomUUID (already relied on elsewhere for run
// ids) so there's no extra dependency. This is a stable identity assigned once
// at creation — unlike a timestamp, which can collide and mutates when an entry
// is promoted (dedup move-to-top).
export function newId(): string {
  return crypto.randomUUID();
}
