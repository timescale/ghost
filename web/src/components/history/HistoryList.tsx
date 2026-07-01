import { type ReactNode, useState } from 'react';

import { Icon } from '../Icon';

interface RowProps {
  // Whether this row is the selected one.
  active: boolean;
  // Select this row (whole-row click / Enter / Space).
  onSelect: () => void;
  // Remove this row (after inline confirmation).
  onRemove: () => void;
  // aria-label/title for the remove + confirm controls (e.g. "Remove from
  // history", "Delete run (evict from cache)").
  removeLabel: string;
  // The row's content (preview + metadata), laid out in the flexible column.
  children: ReactNode;
}

// HistoryListRow is the shared left-column row for every history panel: a
// whole-row selectable target (with keyboard support) plus a hover-revealed,
// two-step confirm delete. The nested delete/confirm buttons stop propagation
// so they don't also select the row. Confirmation state is local to the row.
export function HistoryListRow({
  active,
  onSelect,
  onRemove,
  removeLabel,
  children,
}: RowProps) {
  const [confirming, setConfirming] = useState(false);

  return (
    <li>
      {/* biome-ignore lint/a11y/useSemanticElements: a native <button> can't be used because the row contains nested action buttons (remove/confirm), which is invalid HTML; the role/tabIndex/keydown handler provide equivalent button semantics */}
      <div
        role="button"
        tabIndex={0}
        onClick={onSelect}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onSelect();
          }
        }}
        className={`group flex w-full cursor-pointer items-center gap-2 border-b border-slate-100 px-3 py-2 text-left ${
          active ? 'bg-slate-100' : 'hover:bg-slate-50'
        }`}
      >
        <span className="flex min-w-0 flex-1 flex-col items-start">
          {children}
        </span>
        {confirming ? (
          <span className="flex items-center gap-1">
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onRemove();
                setConfirming(false);
              }}
              aria-label={`Confirm: ${removeLabel}`}
              title={`Confirm: ${removeLabel}`}
              className="rounded border border-red-300 bg-red-50 p-1 text-red-600 hover:bg-red-100"
            >
              <Icon name="check" size="sm" color="current" />
            </button>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setConfirming(false);
              }}
              aria-label="Cancel"
              title="Cancel"
              className="rounded border border-slate-300 bg-white p-1 text-slate-600 hover:bg-slate-50"
            >
              <Icon name="x" size="sm" color="current" />
            </button>
          </span>
        ) : (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              setConfirming(true);
            }}
            aria-label={removeLabel}
            title={removeLabel}
            className="rounded p-1 text-slate-400 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-slate-200 hover:text-red-600"
          >
            <Icon name="trash" size="sm" color="current" />
          </button>
        )}
      </div>
    </li>
  );
}

interface FooterProps {
  // Confirmation prompt + button label (e.g. "Clear all editor history").
  label: string;
  // Empty the entire history.
  onClear: () => void;
}

// ClearHistoryFooter is the shared "Clear all …" footer with a two-step
// confirmation, pinned to the bottom of a history panel's left column.
export function ClearHistoryFooter({ label, onClear }: FooterProps) {
  const [confirming, setConfirming] = useState(false);

  return (
    <div className="border-t border-slate-200 p-2">
      {confirming ? (
        <div className="flex items-center justify-between gap-2 text-xs text-slate-600">
          <span>{label}?</span>
          <span className="flex gap-1">
            <button
              type="button"
              onClick={() => {
                onClear();
                setConfirming(false);
              }}
              className="rounded border border-red-300 bg-red-50 px-2 py-1 text-red-600 hover:bg-red-100"
            >
              Clear
            </button>
            <button
              type="button"
              onClick={() => setConfirming(false)}
              className="rounded border border-slate-300 bg-white px-2 py-1 text-slate-600 hover:bg-slate-50"
            >
              Cancel
            </button>
          </span>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setConfirming(true)}
          className="flex w-full items-center justify-center gap-1.5 rounded border border-slate-300 bg-white px-2 py-1.5 text-xs text-slate-600 hover:bg-slate-50 hover:text-slate-800"
        >
          <Icon name="trash" size="sm" color="current" />
          {label}
        </button>
      )}
    </div>
  );
}
