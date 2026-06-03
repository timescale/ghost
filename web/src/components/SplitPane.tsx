import { useCallback, useEffect, useRef } from 'react';

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
  const draggingRef = useRef(false);

  const onMouseDown = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    e.preventDefault();
    draggingRef.current = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }, []);

  useEffect(() => {
    const onMouseMove = (e: MouseEvent) => {
      if (!draggingRef.current || !containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      onLeftWidthChange(e.clientX - rect.left);
    };
    const onMouseUp = () => {
      if (!draggingRef.current) return;
      draggingRef.current = false;
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    return () => {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    };
  }, [onLeftWidthChange]);

  return (
    <div ref={containerRef} className="flex h-full w-full flex-auto">
      {showLeft ? (
        <>
          <div
            style={{
              width: leftWidth,
              minWidth: minLeftWidth,
              maxWidth: maxLeftWidth,
            }}
            className="flex h-full shrink-0 flex-col overflow-hidden border-r border-slate-200 bg-white"
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
            className="group h-full w-1 shrink-0 cursor-col-resize bg-slate-100 hover:bg-blue-400"
          />
        </>
      ) : null}
      <div className="flex h-full flex-auto flex-col overflow-hidden">
        {right}
      </div>
    </div>
  );
}
