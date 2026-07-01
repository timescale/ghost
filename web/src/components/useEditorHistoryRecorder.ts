import { useEffect, useMemo, useRef } from 'react';

import { useServeStore } from '../store';
import { debounce } from '../util/debounce';

// Debounce window for recording the editor contents into editor history after
// the user last changed them. Long enough that only drafts the user dwells on
// are captured — not every intermediate keystroke while actively typing.
// Matches the chart config recorder's window for consistency.
const RECORD_DEBOUNCE_MS = 1500;

// useEditorHistoryRecorder records the full editor contents into editor history
// as the user edits — never the content merely loaded on startup (or applied
// from history), which seeds the baseline. A change from the baseline that
// settles for RECORD_DEBOUNCE_MS is recorded; the store additionally dedups
// globally, so returning to an earlier draft promotes it rather than
// duplicating it.
//
// `initialSql` is the editor content when the hook mounts (from persisted state
// or empty). It seeds the baseline synchronously, so the first draft the user
// authors is recorded even if they edit within the debounce window, while the
// loaded content itself isn't re-recorded.
export function useEditorHistoryRecorder(sql: string): void {
  const addEditorHistoryEntry = useServeStore((s) => s.addEditorHistoryEntry);

  // The content we won't record (loaded/just-recorded). Seeded synchronously
  // from the content present when the hook mounts. useRef's initializer runs
  // only on mount, so later sql changes don't reset the baseline.
  const baselineRef = useRef<string>(sql);

  const record = useMemo(
    () =>
      debounce((next: string) => {
        if (next.trim() === baselineRef.current.trim()) return;
        baselineRef.current = next;
        addEditorHistoryEntry(next);
      }, RECORD_DEBOUNCE_MS),
    [addEditorHistoryEntry],
  );

  // Feed every editor change through the debounced recorder.
  useEffect(() => {
    record(sql);
  }, [sql, record]);

  // Cancel any pending record on unmount.
  useEffect(() => record.cancel, [record]);
}
