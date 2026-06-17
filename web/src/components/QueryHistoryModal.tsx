import { useMemo, useState } from 'react';

import { type QueryHistoryEntry, useServeStore } from '../store';
import { formatAbsoluteTime, formatRelativeTime } from '../util/time';
import { Icon } from './Icon';
import { Modal } from './Modal';
import { SqlCodeView } from './SqlCodeView';

interface Props {
  onClose: () => void;
  // Replace the entire editor contents with the given SQL, then close.
  onApply: (sql: string) => void;
  // Append the given SQL to the editor contents, then close.
  onAppend: (sql: string) => void;
}

// A one-line preview of an entry's SQL for the list (whitespace collapsed).
function previewSql(sql: string): string {
  return sql.trim().replace(/\s+/g, ' ');
}

// Total number of times the entry's SQL was run (the entry itself plus any
// deduplicated consecutive runs).
function runCount(entry: QueryHistoryEntry): number {
  return 1 + (entry.additionalRuns?.length ?? 0);
}

// QueryHistoryModal lists previously run queries (newest first). Selecting one
// shows its SQL in a read-only viewer on the right, with actions to copy it,
// apply it to the editor (replace or append), or remove it from the history. A
// "Clear all" button (with confirmation) empties the entire history.
export function QueryHistoryModal({ onClose, onApply, onAppend }: Props) {
  const history = useServeStore((s) => s.queryHistory);
  const removeEntry = useServeStore((s) => s.removeQueryHistoryEntry);
  const clearHistory = useServeStore((s) => s.clearQueryHistory);

  const [selectedIndex, setSelectedIndex] = useState(0);
  const [confirmingClear, setConfirmingClear] = useState(false);
  // Index of the entry whose delete is awaiting inline confirmation, if any.
  const [confirmingRemove, setConfirmingRemove] = useState<number | null>(null);

  // Clamp the selection to the current list so removals don't leave it dangling.
  const activeIndex = Math.min(selectedIndex, Math.max(0, history.length - 1));
  const selected = history[activeIndex];

  // Recompute "now" once per render so all relative times share a reference.
  const now = useMemo(() => Date.now(), []);

  const handleRemove = (index: number) => {
    removeEntry(index);
    if (index < selectedIndex) setSelectedIndex((i) => i - 1);
    setConfirmingRemove(null);
  };

  return (
    <Modal onClose={onClose} className="h-[80vh] w-[min(1100px,92vw)]">
      <div className="flex items-center justify-between border-b border-slate-200 px-4 py-2">
        <span className="text-sm font-semibold text-slate-900">
          Query history
        </span>
        <button
          type="button"
          onClick={onClose}
          className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700"
          aria-label="Close"
        >
          ✕
        </button>
      </div>

      {history.length === 0 ? (
        <div className="flex flex-1 items-center justify-center p-8 text-sm text-slate-500">
          No query history yet. Queries you run will appear here.
        </div>
      ) : (
        <div className="flex min-h-0 flex-1">
          {/* Left: list of entries (newest first) plus the clear-all action. */}
          <div className="flex w-80 min-w-72 flex-col border-r border-slate-200">
            <ul className="min-h-0 flex-1 overflow-auto">
              {history.map((entry, index) => {
                const active = index === activeIndex;
                const runs = runCount(entry);
                return (
                  <li
                    // History entries have no stable id; index is the natural,
                    // stable key for this newest-first list.
                    // biome-ignore lint/suspicious/noArrayIndexKey: list keyed by position
                    key={index}
                  >
                    {/* The whole row is the clickable target; nested buttons
                        (delete/confirm) stop propagation so they don't also
                        select the row. */}
                    {/* biome-ignore lint/a11y/useSemanticElements: a native <button> can't be used because the row contains nested action buttons (remove/confirm), which is invalid HTML; the role/tabIndex/keydown handler provide equivalent button semantics */}
                    <div
                      role="button"
                      tabIndex={0}
                      onClick={() => setSelectedIndex(index)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          setSelectedIndex(index);
                        }
                      }}
                      className={`group flex w-full cursor-pointer items-center gap-2 border-b border-slate-100 px-3 py-2 text-left ${
                        active ? 'bg-slate-100' : 'hover:bg-slate-50'
                      }`}
                    >
                      <span className="flex min-w-0 flex-1 flex-col items-start">
                        <span className="flex w-full items-center gap-1.5">
                          <Icon
                            name={entry.success ? 'check' : 'x'}
                            size="xs"
                            color={entry.success ? 'green' : 'red'}
                          />
                          <span
                            className="truncate font-mono text-xs text-slate-700"
                            title={previewSql(entry.sql)}
                          >
                            {previewSql(entry.sql)}
                          </span>
                        </span>
                        <span className="mt-0.5 flex items-center gap-1.5 text-[11px] text-slate-400">
                          <span title={formatAbsoluteTime(entry.ts)}>
                            {formatRelativeTime(entry.ts, now)}
                          </span>
                          {runs > 1 ? <span>· {runs}×</span> : null}
                        </span>
                      </span>
                      {confirmingRemove === index ? (
                        <span className="flex items-center gap-1">
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              handleRemove(index);
                            }}
                            aria-label="Confirm remove"
                            title="Confirm remove"
                            className="rounded border border-red-300 bg-red-50 p-1 text-red-600 hover:bg-red-100"
                          >
                            <Icon name="check" size="sm" color="current" />
                          </button>
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              setConfirmingRemove(null);
                            }}
                            aria-label="Cancel remove"
                            title="Cancel remove"
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
                            setConfirmingRemove(index);
                          }}
                          aria-label="Remove from history"
                          title="Remove from history"
                          className="rounded p-1 text-slate-400 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-slate-200 hover:text-red-600"
                        >
                          <Icon name="trash" size="sm" color="current" />
                        </button>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
            <div className="border-t border-slate-200 p-2">
              {confirmingClear ? (
                <div className="flex items-center justify-between gap-2 text-xs text-slate-600">
                  <span>Clear all history?</span>
                  <span className="flex gap-1">
                    <button
                      type="button"
                      onClick={() => {
                        clearHistory();
                        setConfirmingClear(false);
                      }}
                      className="rounded border border-red-300 bg-red-50 px-2 py-1 text-red-600 hover:bg-red-100"
                    >
                      Clear
                    </button>
                    <button
                      type="button"
                      onClick={() => setConfirmingClear(false)}
                      className="rounded border border-slate-300 bg-white px-2 py-1 text-slate-600 hover:bg-slate-50"
                    >
                      Cancel
                    </button>
                  </span>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmingClear(true)}
                  className="flex w-full items-center justify-center gap-1.5 rounded border border-slate-300 bg-white px-2 py-1.5 text-xs text-slate-600 hover:bg-slate-50 hover:text-slate-800"
                >
                  <Icon name="trash" size="sm" color="current" />
                  Clear all history
                </button>
              )}
            </div>
          </div>

          {/* Right: read-only viewer of the selected entry. The append/replace
              actions live in the viewer's own toolbar, alongside its copy
              button. */}
          <div className="flex min-w-0 flex-1 flex-col">
            <div className="flex min-h-0 flex-1 flex-col p-2">
              {selected ? (
                <SqlCodeView
                  query={selected.sql}
                  fill
                  toolbarActions={
                    <>
                      <button
                        type="button"
                        onClick={() => onAppend(selected.sql)}
                        className="rounded border border-slate-300 bg-white px-2 py-1 text-xs text-slate-600 hover:bg-slate-50 hover:text-slate-800"
                      >
                        Append to editor
                      </button>
                      <button
                        type="button"
                        onClick={() => onApply(selected.sql)}
                        className="rounded border border-slate-800 bg-slate-800 px-2 py-1 text-xs text-white hover:bg-slate-700"
                      >
                        Replace editor
                      </button>
                    </>
                  }
                />
              ) : null}
            </div>
          </div>
        </div>
      )}
    </Modal>
  );
}
