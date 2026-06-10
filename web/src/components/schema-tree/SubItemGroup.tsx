import type { ReactNode } from 'react';

import { TreeRow } from './rows';
import { filterForSearch } from './search';
import type { TreeContext } from './TreeContext';

interface SubItemGroupProps<T extends { name: string }> {
  ctx: TreeContext;
  // Node key of the parent table/view this group belongs to.
  parentKey: string;
  // Key segment for this group (e.g. 'columns', 'indexes'). Must stay in
  // lockstep with computeSearch's sub-item keys.
  subKind: string;
  label: string;
  items: T[];
  // Overrides the key-segment derivation for items whose node-key name
  // differs from `name` (e.g. partitionNodeName).
  getName?: (item: T) => string;
  // renderItem must set a React key on the returned element.
  renderItem: (item: T) => ReactNode;
}

// SubItemGroup renders a collapsible group of sub-items under a table or
// view node (Columns, Partitions, Constraints, Indexes, Triggers). When a
// search is active, only the sub-items that themselves match are rendered —
// popsql does the same: searching for "plan" inside a multi-column table
// collapses Columns down to just the matching ones. The group disappears
// entirely when it has nothing to show.
export function SubItemGroup<T extends { name: string }>({
  ctx,
  parentKey,
  subKind,
  label,
  items,
  getName,
  renderItem,
}: SubItemGroupProps<T>) {
  const visible = filterForSearch(ctx, parentKey, subKind, items, getName);
  if (visible.length === 0) return null;
  return (
    <TreeRow
      ctx={ctx}
      nodeKey={`${parentKey}/${subKind}`}
      label={label}
      depth={3}
      hasChildren
      count={ctx.searchActive ? visible.length : items.length}
    >
      {visible.map(renderItem)}
    </TreeRow>
  );
}
