import type { MouseEvent } from 'react';

import type { ContextMenuState, MenuItem } from '../ContextMenu';

// ModalFormat selects how the shared modal renders its text: as
// syntax-highlighted SQL (definitions, trigger statements, partition bounds)
// or as plain prose (object comments).
export type ModalFormat = 'sql' | 'text';

// ShowModal opens the shared definition modal with a title and text.
// format defaults to 'sql'.
export type ShowModal = (
  title: string,
  text: string,
  format?: ModalFormat,
) => void;

// TreeContext carries the shared tree state and callbacks down through the
// node renderers, so each node component doesn't need a dozen props.
export interface TreeContext {
  expanded: Set<string>;
  // Nodes the user has explicitly collapsed while a search is active. During
  // search every rendered node is expanded by default (matching popsql), so we
  // only need to track the exceptions. This set is transient: it lives in
  // component state and is reset whenever the search term changes, so toggling
  // a node while searching never mutates the persisted `expanded` state of the
  // unfiltered tree.
  collapsedDuringSearch: Set<string>;
  searchActive: boolean;
  searchMatches: Set<string> | null;
  searchTerm: string;
  toggle: (key: string) => void;
  setContextMenu: (m: ContextMenuState | null) => void;
  showModal: ShowModal;
}

// Whether an expandable node should render its children. While a search is
// active every node is expanded by default (so all matches are visible) unless
// the user has explicitly collapsed it; otherwise we consult the persisted
// expanded state.
export function nodeExpanded(ctx: TreeContext, nodeKey: string): boolean {
  return ctx.searchActive
    ? !ctx.collapsedDuringSearch.has(nodeKey)
    : ctx.expanded.has(nodeKey);
}

// contextMenuHandler builds an onContextMenu handler that opens the tree's
// context menu at the cursor position. `buildItems` is a thunk so the menu
// items are only constructed when the menu actually opens.
export function contextMenuHandler(
  ctx: TreeContext,
  buildItems: () => MenuItem[],
): (e: MouseEvent<HTMLDivElement>) => void {
  return (e) => {
    e.preventDefault();
    ctx.setContextMenu({ x: e.clientX, y: e.clientY, items: buildItems() });
  };
}
