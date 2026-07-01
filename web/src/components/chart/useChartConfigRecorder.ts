import { useCallback, useEffect, useMemo, useRef } from 'react';

import { useServeStore } from '../../store';
import { debounce } from '../../util/debounce';

// Debounce window for recording a chart config into history after it last
// rendered successfully. Deliberately longer than the live render debounce
// (200ms) and the state-persist debounce (400ms), so only configs the user
// actually dwells on are recorded — not every valid intermediate state while
// actively editing.
const RECORD_DEBOUNCE_MS = 1500;

export interface ChartConfigRecorder {
  // Pass to ChartView's onRenderSuccess: signals that the given config rendered
  // cleanly against real rows. Debounced, then records (see below).
  recordRenderSuccess: (config: string) => void;
  // Call when a config is applied programmatically (from history) so it isn't
  // mistaken for a user edit and re-recorded.
  markApplied: (config: string) => void;
}

// useChartConfigRecorder records chart configs into history, but only ones the
// user authored by editing — never the config that's merely loaded on startup
// or restored from history. It works off a "baseline" (the last config that was
// loaded/applied/recorded): a render success records the current config only if
// it differs from the baseline. The store additionally deduplicates globally,
// so a config matching an existing entry is promoted rather than duplicated.
//
// `initialConfig` is the config loaded when the hook mounts (from persisted
// state or the default). It seeds the baseline synchronously, so the very first
// config the user authors is recorded even if they edit it before the first
// render or within the record debounce. (Seeding lazily on the first render
// would instead swallow that first edited config as the baseline.)
export function useChartConfigRecorder(
  initialConfig: string,
): ChartConfigRecorder {
  const addChartConfigHistoryEntry = useServeStore(
    (s) => s.addChartConfigHistoryEntry,
  );

  // The config we won't record (loaded/applied/just-recorded). Seeded
  // synchronously from the config present when the hook mounts, so a user edit
  // made before the first render is still recognized as a change from baseline.
  // useRef's initializer runs only on mount, so later initialConfig changes
  // (e.g. live edits flowing back through props) don't reset the baseline.
  const baselineRef = useRef<string>(initialConfig);

  const recordRenderSuccess = useMemo(
    () =>
      // Record the config that actually rendered (passed in by ChartView), not
      // whatever is in the store when the debounce fires — the user may have
      // edited it to something invalid/unrendered in the meantime.
      debounce((config: string) => {
        if (config.trim() === baselineRef.current.trim()) return;
        baselineRef.current = config;
        addChartConfigHistoryEntry(config);
      }, RECORD_DEBOUNCE_MS),
    [addChartConfigHistoryEntry],
  );

  const markApplied = useCallback(
    (config: string) => {
      // Flush (don't cancel) any pending record so a config the user authored
      // just before applying is committed to history first (against its own
      // contents), then reset the baseline to the applied config so the
      // replayed config isn't re-recorded as a fresh edit.
      recordRenderSuccess.flush();
      baselineRef.current = config;
    },
    [recordRenderSuccess],
  );

  // Flush any pending record on unmount so a config authored within the
  // debounce window is still committed to history rather than dropped.
  useEffect(() => recordRenderSuccess.flush, [recordRenderSuccess]);

  return { recordRenderSuccess, markApplied };
}
