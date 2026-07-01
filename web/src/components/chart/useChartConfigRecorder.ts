import { useServeStore } from '../../store';
import {
  type HistoryRecorder,
  useHistoryRecorder,
} from '../history/useHistoryRecorder';

export interface ChartConfigRecorder {
  // Pass to ChartView's onRenderSuccess: signals that the given config rendered
  // cleanly against real rows. Debounced, then records it (unless it matches
  // the baseline).
  recordRenderSuccess: (config: string) => void;
  // Call when a config is applied programmatically (from history) so it isn't
  // mistaken for a user edit and re-recorded.
  markApplied: HistoryRecorder['markApplied'];
}

// useChartConfigRecorder records chart configs into history, but only ones the
// user authored by editing — never the config that's merely loaded on startup
// or restored from history. It works off a "baseline" (the last config that was
// loaded/applied/recorded): a render success records the current config only if
// it differs from the baseline. The store additionally deduplicates globally,
// so a config matching an existing entry is promoted rather than duplicated.
//
// `initialConfig` seeds the baseline synchronously (see useHistoryRecorder), so
// the very first config the user authors is recorded even if edited before the
// first render or within the record debounce. Unlike the editor recorder, the
// trigger isn't every change but ChartView's render-success callback — a config
// is only recorded once it has actually rendered against real rows.
export function useChartConfigRecorder(
  initialConfig: string,
): ChartConfigRecorder {
  const addChartConfigHistoryEntry = useServeStore(
    (s) => s.addChartConfigHistoryEntry,
  );
  const { record, markApplied } = useHistoryRecorder(
    initialConfig,
    addChartConfigHistoryEntry,
  );

  return { recordRenderSuccess: record, markApplied };
}
