import { useState } from 'react';

// useHistorySelection owns the "selected row" state shared by every history
// panel's list. It clamps the active index to the current item count (so a
// removal or eviction that shortens the list can't leave the selection dangling
// past the end) and adjusts the stored index when an earlier row is removed.
export function useHistorySelection(itemCount: number) {
  const [selectedIndex, setSelectedIndex] = useState(0);

  // Clamp so trimming/eviction can't point the selection past the end.
  const activeIndex = Math.min(selectedIndex, Math.max(0, itemCount - 1));

  // Keep the same row selected after removing one before it (indices shift
  // down by one); removing the selected row or a later one needs no change,
  // since clamping handles the end of the list.
  const adjustForRemoval = (removedIndex: number) => {
    if (removedIndex < selectedIndex) setSelectedIndex((i) => i - 1);
  };

  return { activeIndex, setSelectedIndex, adjustForRemoval };
}
