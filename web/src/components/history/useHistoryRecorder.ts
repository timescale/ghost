import { useCallback, useEffect, useMemo, useRef } from 'react';

import { debounce } from '../../util/debounce';

// Debounce window for recording content into a history (editor drafts or chart
// configs) after it last changed. Long enough that only content the user dwells
// on is captured — not every intermediate keystroke/edit while actively working.
export const RECORD_DEBOUNCE_MS = 1500;

export interface HistoryRecorder {
  // Feed content through the debounced recorder. Records it (via the store
  // action) once it settles for RECORD_DEBOUNCE_MS, unless it matches the
  // current baseline (loaded/applied/just-recorded content, which shouldn't be
  // re-recorded).
  record: (content: string) => void;
  // Call before replaying content from a history panel (Open in editor,
  // Replace/Append, Apply config) so it isn't re-recorded as a fresh edit —
  // it's already an entry, and re-recording would just churn its ordering.
  // Flushes (doesn't drop) any pending record first, so a draft authored just
  // before applying is still committed to history, then resets the baseline to
  // the applied content.
  markApplied: (content: string) => void;
}

// useHistoryRecorder is the shared engine behind editor-draft and chart-config
// history recording. It works off a "baseline" (the last content that was
// loaded/applied/recorded): content fed through `record` is recorded only if it
// differs (whitespace-insensitively) from the baseline. The store action passed
// in additionally dedups globally, so returning to earlier content promotes it
// rather than duplicating it.
//
// `initial` seeds the baseline synchronously, so the first content the user
// authors is recorded even if edited within the debounce window, while the
// loaded content itself isn't re-recorded. useRef's initializer runs only on
// mount, so later `initial` changes (live edits flowing back through props)
// don't reset the baseline.
export function useHistoryRecorder(
  initial: string,
  addEntry: (content: string) => void,
): HistoryRecorder {
  // The content we won't record (loaded/applied/just-recorded).
  const baselineRef = useRef<string>(initial);

  const debounced = useMemo(
    () =>
      // Record the content that was actually passed in (captured as the
      // debounce's args), not whatever is current when the debounce fires — the
      // source may have changed to something transient in the meantime.
      debounce((content: string) => {
        if (content.trim() === baselineRef.current.trim()) return;
        baselineRef.current = content;
        addEntry(content);
      }, RECORD_DEBOUNCE_MS),
    [addEntry],
  );

  // Flush any pending record on unmount (e.g. a database switch) so content
  // authored within the debounce window is committed to history, not dropped.
  useEffect(() => debounced.flush, [debounced]);

  const markApplied = useCallback(
    (applied: string) => {
      // Flush (don't cancel) so a draft/config authored just before applying is
      // committed against its own contents first, then reset the baseline to
      // the applied content so the replayed content isn't re-recorded.
      debounced.flush();
      baselineRef.current = applied;
    },
    [debounced],
  );

  return { record: debounced, markApplied };
}
