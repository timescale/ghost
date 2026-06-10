import { useCallback, useEffect, useMemo, useState } from 'react';

import { useServeStore } from '../../store';

const EMPTY_LIST: string[] = [];

// useSchemaTreeExpansion manages the two layers of expand/collapse state for
// a database's schema tree:
//
// - `expanded`: the persisted per-database expansion state from the store.
// - `collapsedDuringSearch`: transient per-search collapse overrides. While a
//   search is active every rendered node is expanded by default, so only the
//   explicit exceptions are tracked. The set is reset whenever the search
//   term changes so a fresh search always starts fully expanded, and so
//   collapses made during one search never carry over to the next (or to the
//   unfiltered view).
//
// `toggleNode` routes to the right layer: while searching it adjusts the
// transient collapse set; otherwise it mutates the persisted expanded state.
export function useSchemaTreeExpansion(databaseId: string, searchTerm: string) {
  const expandedList = useServeStore(
    (s) => s.schemaTreeExpanded[databaseId] ?? EMPTY_LIST,
  );
  const toggle = useServeStore((s) => s.toggleSchemaNode);
  const toggleForDb = useCallback(
    (key: string) => toggle(databaseId, key),
    [toggle, databaseId],
  );

  const expanded = useMemo(() => new Set(expandedList), [expandedList]);
  const searchActive = searchTerm.length > 0;

  const [collapsedDuringSearch, setCollapsedDuringSearch] = useState<
    Set<string>
  >(() => new Set());

  // biome-ignore lint/correctness/useExhaustiveDependencies: searchTerm is the reset trigger, not read in the body
  useEffect(() => {
    setCollapsedDuringSearch(new Set());
  }, [searchTerm]);

  const toggleCollapsedDuringSearch = useCallback((key: string) => {
    setCollapsedDuringSearch((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const toggleNode = useCallback(
    (key: string) => {
      if (searchActive) toggleCollapsedDuringSearch(key);
      else toggleForDb(key);
    },
    [searchActive, toggleCollapsedDuringSearch, toggleForDb],
  );

  return { expanded, collapsedDuringSearch, searchActive, toggleNode };
}
