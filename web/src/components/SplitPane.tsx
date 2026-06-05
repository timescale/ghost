import { useCallback, useEffect, useRef, useState } from 'react';

interface SplitPaneProps {
  leftWidth: number;
  minLeftWidth: number;
  maxLeftWidth: number;
  showLeft: boolean;
  onLeftWidthChange: (width: number) => void;
  left: React.ReactNode;
  right: React.ReactNode;
}

// SplitPane lays out two panes side-by-side with a draggable 4px divider.
// The left pane width is controlled by the parent; the divider drag emits
// onLeftWidthChange. Hiding the left pane collapses it (and the divider) out
// of layout entirely without unmounting children.
export function SplitPane({
  leftWidth,
  minLeftWidth,
  maxLeftWidth,
  showLeft,
  onLeftWidthChange,
  left,
  right,
}: SplitPaneProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [dragging, setDragging] = useState(false);
  const draggingRef = useRef(false);
  const onLeftWidthChangeRef = useRef(onLeftWidthChange);
  onLeftWidthChangeRef.current = onLeftWidthChange;

  const onMouseDown = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    e.preventDefault();
    draggingRef.current = true;
    setDragging(true);
  }, []);

  // Keyboard resize for accessibility: the divider exposes a separator role
  // with aria-valuemin/max/now, so arrow keys must adjust the width. Shift
  // takes larger steps.
  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const step = e.shiftKey ? 32 : 8;
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        onLeftWidthChangeRef.current(leftWidth - step);
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        onLeftWidthChangeRef.current(leftWidth + step);
      }
    },
    [leftWidth],
  );

  useEffect(() => {
    const onMouseMove = (e: MouseEvent) => {
      if (!draggingRef.current || !containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      onLeftWidthChangeRef.current(e.clientX - rect.left);
    };
    const onMouseUp = () => {
      if (!draggingRef.current) return;
      draggingRef.current = false;
      setDragging(false);
    };
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    return () => {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    };
  }, []);

  return (
    <div ref={containerRef} className="flex relative h-full w-full flex-auto">
      {showLeft ? (
        <>
          <div
            style={{
              width: leftWidth,
              minWidth: minLeftWidth,
              maxWidth: maxLeftWidth,
            }}
            className="flex h-full flex-0 flex-col overflow-hidden border-r border-slate-200 bg-white"
          >
            {left}
          </div>
          {/* biome-ignore lint/a11y/useSemanticElements: separator role is the correct ARIA role for a draggable splitter handle */}
          <div
            role="separator"
            aria-orientation="vertical"
            aria-valuemin={minLeftWidth}
            aria-valuemax={maxLeftWidth}
            aria-valuenow={leftWidth}
            tabIndex={0}
            onMouseDown={onMouseDown}
            onKeyDown={onKeyDown}
            className={`group h-full w-1 shrink-0 cursor-col-resize bg-slate-100 hover:bg-blue-400 ${dragging ? 'bg-blue-400' : ''}`}
          />
        </>
      ) : null}
      <div className="flex h-full flex-auto flex-col overflow-hidden">
        {right}
      </div>
      {dragging && <div className="absolute inset-0 cursor-col-resize" />}
    </div>
  );
}
