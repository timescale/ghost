import type { MouseEvent, ReactNode } from 'react';

import { Icon } from '../Icon';
import { nodeExpanded, type ShowModal, type TreeContext } from './TreeContext';

// Indent layout matches popsql: each indent column is a fixed-width span
// carrying the vertical guide line on its right edge. The guide line sits at
// half the caret slot width, which centers it under the ancestor chevron.
const CARET_PX = 12;
const INDENT_STEP_PX = 20;
const INDENT_PAD = CARET_PX / 2;
const INDENT_GAP = INDENT_STEP_PX - INDENT_PAD;

interface SchemaRootRowProps {
  ctx: TreeContext;
  nodeKey: string;
  label: ReactNode;
  hasChildren: boolean;
  rightDetail?: ReactNode;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
  children?: ReactNode;
}

// Schema row: bold, no left caret, with a hover-revealed right caret. Matches
// popsql's root-level treatment.
export function SchemaRootRow({
  nodeKey,
  label,
  hasChildren,
  rightDetail,
  onContextMenu,
  children,
  ctx,
}: SchemaRootRowProps) {
  const isExpanded = nodeExpanded(ctx, nodeKey);
  return (
    <>
      <div
        role={hasChildren ? 'button' : undefined}
        tabIndex={hasChildren ? 0 : undefined}
        className="group flex h-[24px] min-w-0 cursor-default items-center gap-1 px-2 font-semibold text-slate-900 hover:bg-slate-100"
        onClick={hasChildren ? () => ctx.toggle(nodeKey) : undefined}
        onKeyDown={
          hasChildren
            ? (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  ctx.toggle(nodeKey);
                }
              }
            : undefined
        }
        onContextMenu={onContextMenu}
      >
        <span className="min-w-0 truncate">{label}</span>
        {hasChildren ? (
          <Icon
            name="chevron-down"
            className={`flex-none text-slate-400 opacity-0 transition-transform group-hover:opacity-100 ${
              isExpanded ? '' : '-rotate-90'
            }`}
            size={CARET_PX}
          />
        ) : null}
        {rightDetail ? <RightDetail>{rightDetail}</RightDetail> : null}
      </div>
      {isExpanded ? children : null}
    </>
  );
}

interface TreeRowProps {
  ctx: TreeContext;
  nodeKey: string;
  label: ReactNode;
  depth: number;
  icon?: ReactNode;
  count?: number;
  rightDetail?: ReactNode;
  hasChildren?: boolean;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
  children?: ReactNode;
}

// TreeRow renders any non-leaf, non-root node: a group header (Tables,
// Columns, Indexes…) or an expandable item (a table). It always reserves a
// caret slot on the left so siblings stay aligned regardless of icon width.
export function TreeRow({
  ctx,
  nodeKey,
  label,
  depth,
  icon,
  count,
  rightDetail,
  hasChildren,
  onContextMenu,
  children,
}: TreeRowProps) {
  const isExpanded = nodeExpanded(ctx, nodeKey);
  const onClick = hasChildren ? () => ctx.toggle(nodeKey) : undefined;
  return (
    <>
      <RowShell
        depth={depth}
        onClick={onClick}
        onContextMenu={onContextMenu}
        clickable={!!hasChildren}
      >
        <CaretSlot expanded={isExpanded} hasChildren={!!hasChildren} />
        {icon ? <span className="flex-none text-slate-500">{icon}</span> : null}
        <span className="min-w-0 truncate text-slate-700">{label}</span>
        {typeof count === 'number' ? <Pill>{count}</Pill> : null}
        {rightDetail ? <RightDetail>{rightDetail}</RightDetail> : null}
      </RowShell>
      {isExpanded ? children : null}
    </>
  );
}

interface LeafRowProps {
  label: ReactNode;
  depth: number;
  rightDetail?: ReactNode;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
}

// LeafRow is for terminal nodes (column, index, trigger, routine, enum).
// popsql does not reserve an empty caret slot for leaves; the leaf label
// starts where a sibling expandable row's caret would start.
export function LeafRow({
  label,
  depth,
  rightDetail,
  onContextMenu,
}: LeafRowProps) {
  return (
    <RowShell depth={depth} onContextMenu={onContextMenu} clickable={false}>
      <span className="min-w-0 truncate text-slate-700">{label}</span>
      {rightDetail ? <RightDetail>{rightDetail}</RightDetail> : null}
    </RowShell>
  );
}

interface RowShellProps {
  depth: number;
  clickable: boolean;
  onClick?: () => void;
  onContextMenu?: (e: MouseEvent<HTMLDivElement>) => void;
  children: ReactNode;
}

// RowShell lays out a single tree row: N IndentGuide spans, then the row
// content (caret + icon + label + pills). The indent guides carry the
// vertical connector lines on their right edge, popsql-style.
function RowShell({
  depth,
  clickable,
  onClick,
  onContextMenu,
  children,
}: RowShellProps) {
  return (
    <div
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      className="group/row flex h-[24px] min-w-0 cursor-default items-center gap-1.5 pl-2 pr-2 hover:bg-slate-100"
      onClick={onClick}
      onKeyDown={
        clickable && onClick
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onClick();
              }
            }
          : undefined
      }
      onContextMenu={onContextMenu}
    >
      <IndentGuides depth={depth} />
      {children}
    </div>
  );
}

function IndentGuides({ depth }: { depth: number }) {
  const guideCount = Math.max(0, depth - 1);
  if (guideCount === 0) return null;
  const guides = [];
  for (let i = 0; i < guideCount; i++) {
    guides.push(
      <span
        key={i}
        aria-hidden
        className="h-[24px] flex-none self-stretch border-r border-slate-200"
        style={{ width: INDENT_PAD, marginRight: INDENT_GAP }}
      />,
    );
  }
  return <>{guides}</>;
}

// CaretSlot reserves a fixed-width column for the chevron so that the row's
// icon/name column starts at the same x-position regardless of whether the
// row has children. The chevron itself rotates -90deg when collapsed.
function CaretSlot({
  expanded,
  hasChildren,
}: {
  expanded: boolean;
  hasChildren: boolean;
}) {
  return (
    <span
      className="flex flex-none items-center justify-center text-slate-400"
      style={{ width: CARET_PX }}
    >
      {hasChildren ? (
        <Icon
          name="chevron-down"
          className={`transition-transform ${expanded ? '' : '-rotate-90'}`}
          size={CARET_PX}
        />
      ) : null}
    </span>
  );
}

// RightDetail wraps trailing pills/badges. It uses `ml-auto` to push itself
// to the right edge of the row when there is free space, and a very high
// `flex-shrink` so that, when horizontal space is tight, the detail column
// shrinks (its trailing content clipping off-screen) far before the row's
// name truncates.
function RightDetail({ children }: { children: ReactNode }) {
  return (
    <span
      className="ml-auto flex min-w-0 items-center justify-end gap-1 overflow-hidden pl-2"
      style={{ flexShrink: 9999 }}
    >
      {children}
    </span>
  );
}

export function Pill({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex flex-none items-center whitespace-nowrap rounded bg-[rgba(0,0,0,0.1)] px-1 py-px text-[11px] leading-tight text-slate-600">
      {children}
    </span>
  );
}

interface CommentBadgeProps {
  // Title for the comment modal (the object's name/signature).
  title: string;
  comment?: string;
  showModal: ShowModal;
}

// CommentBadge renders a small Pill-styled speech-bubble icon when the object
// has a COMMENT. The full comment text is readable on hover (native title
// tooltip), and clicking opens the plain-text comment modal. Renders nothing
// when the object has no comment, so callers can include it unconditionally.
export function CommentBadge({ title, comment, showModal }: CommentBadgeProps) {
  if (!comment) return null;
  return (
    <button
      type="button"
      title={comment}
      aria-label={`View comment for ${title}`}
      // Stop propagation so opening the comment doesn't also toggle the
      // row's expansion (click) — and likewise for Enter/Space when the
      // badge has keyboard focus inside a clickable row (keydown).
      onClick={(e) => {
        e.stopPropagation();
        showModal(title, comment, 'text');
      }}
      onKeyDown={(e) => e.stopPropagation()}
      className="inline-flex flex-none cursor-pointer items-center whitespace-nowrap rounded bg-[rgba(0,0,0,0.1)] px-1 py-px text-[11px] leading-tight text-slate-600 hover:bg-[rgba(0,0,0,0.18)] hover:text-slate-900"
    >
      <Icon name="comment" size={12} />
    </button>
  );
}
