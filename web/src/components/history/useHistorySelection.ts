import { useState } from 'react';

// useHistorySelection owns the "selected row" state shared by every history
// panel's list, keyed by a stable per-entry id rather than a positional index.
// Tracking by id means the selection follows the same entry as the live list
// mutates underneath the open modal — prepends from the debounced recorder or
// an agent authoring via MCP, and removals — instead of silently jumping to a
// different entry when indices shift.
//
// `ids` is the current list of entry ids in display order (newest first). The
// returned `activeId` is the selected entry's id, resolved to a real entry by
// the caller. When the selected entry disappears (removed/evicted) or nothing
// is selected yet, it falls back to the first entry so the detail pane always
// has something to show while the list is non-empty.
export function useHistorySelection(ids: string[]) {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Resolve the selection: keep it if the entry still exists, otherwise fall
  // back to the first (newest) entry, or null when the list is empty.
  const activeId =
    selectedId !== null && ids.includes(selectedId)
      ? selectedId
      : (ids[0] ?? null);

  return { activeId, setSelectedId };
}
