import { type ReactNode, useEffect, useRef } from 'react';

export interface MenuItem {
  key: string;
  label: ReactNode;
  onClick: () => void;
}

export interface ContextMenuState {
  x: number;
  y: number;
  items: MenuItem[];
}

interface Props {
  state: ContextMenuState;
  onClose: () => void;
}

export function ContextMenu({ state, onClose }: Props) {
  const ref = useRef<HTMLDivElement | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const onDown = (e: globalThis.MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onCloseRef.current();
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onCloseRef.current();
      }
    };
    // Defer attaching the outside-click listener by one tick so the same
    // mousedown that opened this menu doesn't immediately close it.
    const id = setTimeout(() => {
      window.addEventListener('mousedown', onDown);
      window.addEventListener('keydown', onKey);
    }, 0);
    return () => {
      clearTimeout(id);
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  }, []);

  return (
    <div
      ref={ref}
      role="menu"
      className="fixed z-50 min-w-[200px] rounded border border-slate-200 bg-white py-1 text-sm shadow-lg"
      style={{ top: state.y, left: state.x }}
    >
      {state.items.map((item) => (
        <button
          key={item.key}
          type="button"
          role="menuitem"
          onClick={() => {
            item.onClick();
            onClose();
          }}
          className="flex w-full items-center gap-2 px-3 py-1 text-left hover:bg-blue-50"
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
