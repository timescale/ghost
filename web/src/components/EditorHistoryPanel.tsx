import { useMemo } from 'react';

import { useServeStore } from '../store';
import { formatAbsoluteTime, formatRelativeTime } from '../util/time';
import { ClearHistoryFooter, HistoryListRow } from './history/HistoryList';
import { useHistorySelection } from './history/useHistorySelection';
import { SqlCodeView } from './SqlCodeView';

interface Props {
  // Replace the entire editor contents with the given SQL, then close.
  onApply: (sql: string) => void;
  // Append the given SQL to the editor contents, then close.
  onAppend: (sql: string) => void;
}

// A one-line preview of an entry's SQL for the list (whitespace collapsed).
function previewSql(sql: string): string {
  return sql.trim().replace(/\s+/g, ' ');
}

// EditorHistoryPanel lists past editor drafts (full editor contents recorded as
// the user edits, newest first). Selecting one shows its SQL in a read-only
// viewer on the right, with actions to copy it, apply it to the editor (replace
// or append), or remove it from the history. A "Clear all" button (with
// confirmation) empties the entire history. Rendered as a tab inside the
// unified HistoryModal.
export function EditorHistoryPanel({ onApply, onAppend }: Props) {
  const history = useServeStore((s) => s.editorHistory);
  const removeEntry = useServeStore((s) => s.removeEditorHistoryEntry);
  const clearHistory = useServeStore((s) => s.clearEditorHistory);

  const { activeIndex, setSelectedIndex, adjustForRemoval } =
    useHistorySelection(history.length);
  const selected = history[activeIndex];

  // Recompute "now" once per render so all relative times share a reference.
  const now = useMemo(() => Date.now(), []);

  const handleRemove = (index: number) => {
    removeEntry(index);
    adjustForRemoval(index);
  };

  if (history.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center p-8 text-sm text-slate-500">
        No editor history yet. Your editor drafts are saved here as you edit.
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: list of entries (newest first) plus the clear-all action. */}
      <div className="flex w-80 min-w-72 flex-col border-r border-slate-200">
        <ul className="min-h-0 flex-1 overflow-auto">
          {history.map((entry, index) => (
            <HistoryListRow
              // History entries have no stable id; index is the natural, stable
              // key for this newest-first list.
              // biome-ignore lint/suspicious/noArrayIndexKey: list keyed by position
              key={index}
              active={index === activeIndex}
              onSelect={() => setSelectedIndex(index)}
              onRemove={() => handleRemove(index)}
              removeLabel="Remove from history"
            >
              <span
                className="w-full truncate font-mono text-xs text-slate-700"
                title={previewSql(entry.sql)}
              >
                {previewSql(entry.sql)}
              </span>
              <span className="mt-0.5 text-[11px] text-slate-400">
                <span title={formatAbsoluteTime(entry.ts)}>
                  {formatRelativeTime(entry.ts, now)}
                </span>
              </span>
            </HistoryListRow>
          ))}
        </ul>
        <ClearHistoryFooter
          label="Clear all editor history"
          onClear={clearHistory}
        />
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
  );
}
