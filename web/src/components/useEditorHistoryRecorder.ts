import { useCallback, useEffect, useMemo, useRef } from 'react';

import { useServeStore } from '../store';
import { debounce } from '../util/debounce';

// Debounce window for recording the editor contents into editor history after
// they last changed. Long enough that only content that's dwelt on is captured
// — not every intermediate keystroke while actively typing. Matches the chart
// config recorder's window for consistency.
const RECORD_DEBOUNCE_MS = 1500;

export interface EditorHistoryRecorder {
  // Call before replaying content from a history panel (Open in editor,
  // Replace/Append from editor history) so it isn't re-recorded as a fresh
  // draft — it's already an entry, and re-recording would just churn its
  // ordering. NOT called for freshly authored content, including agent-authored
  // SQL, which should flow through into history. Mirrors the chart config
  // recorder's markApplied.
  markApplied: (sql: string) => void;
}

// useEditorHistoryRecorder records the full editor contents into editor history
// as they're freshly authored — the user typing, or the agent authoring SQL via
// MCP — but never the content merely loaded on startup or replayed from a
// history panel (both of which seed the baseline via markApplied). A change
// from the baseline that settles for RECORD_DEBOUNCE_MS is recorded; the store
// additionally dedups globally, so returning to an earlier draft promotes it
// rather than duplicating it.
//
// `initialSql` is the editor content when the hook mounts (from persisted state
// or empty). It seeds the baseline synchronously, so the first draft authored
// is recorded even if edited within the debounce window, while the loaded
// content itself isn't re-recorded.
export function useEditorHistoryRecorder(sql: string): EditorHistoryRecorder {
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

  const markApplied = useCallback(
    (applied: string) => {
      // A pending record from edits made just before applying would otherwise
      // fire against the applied content; cancel it and reset the baseline so
      // the replayed content (and the change event it triggers) is skipped.
      record.cancel();
      baselineRef.current = applied;
    },
    [record],
  );

  return { markApplied };
}
