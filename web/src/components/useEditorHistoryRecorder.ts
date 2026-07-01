import { useEffect } from 'react';

import { useServeStore } from '../store';
import {
  type HistoryRecorder,
  useHistoryRecorder,
} from './history/useHistoryRecorder';

export interface EditorHistoryRecorder {
  // Call before replaying content from a history panel (Open in editor,
  // Replace/Append from editor history) so it isn't re-recorded as a fresh
  // draft — it's already an entry, and re-recording would just churn its
  // ordering. NOT called for freshly authored content, including agent-authored
  // SQL, which should flow through into history. Mirrors the chart config
  // recorder's markApplied.
  markApplied: HistoryRecorder['markApplied'];
}

// useEditorHistoryRecorder records the full editor contents into editor history
// as they're freshly authored — the user typing, or the agent authoring SQL via
// MCP — but never the content merely loaded on startup or replayed from a
// history panel (both of which seed the baseline via markApplied). A change
// from the baseline that settles for the record debounce is recorded; the store
// additionally dedups globally, so returning to an earlier draft promotes it
// rather than duplicating it.
//
// `sql` is the current editor content; every change is fed through the shared
// recorder. Its initial value seeds the baseline synchronously (see
// useHistoryRecorder), so the first draft authored is recorded even if edited
// within the debounce window, while the loaded content itself isn't re-recorded.
export function useEditorHistoryRecorder(sql: string): EditorHistoryRecorder {
  const addEditorHistoryEntry = useServeStore((s) => s.addEditorHistoryEntry);
  const { record, markApplied } = useHistoryRecorder(
    sql,
    addEditorHistoryEntry,
  );

  // Feed every editor change through the debounced recorder.
  useEffect(() => {
    record(sql);
  }, [sql, record]);

  return { markApplied };
}
