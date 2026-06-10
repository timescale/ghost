import {
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react';

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

  // Open at the cursor, then clamp into the viewport once we can measure the
  // rendered menu. Without this, right-clicking near the bottom or right edge
  // would push menu items off-screen with no way to reach them. useLayoutEffect
  // runs before paint, so the clamped position is applied without a flicker.
  // state.items is a dependency because reopening at the same coordinates with
  // a taller/shorter item list changes the menu's measured height, so the
  // clamp must recompute.
  const [pos, setPos] = useState({ top: state.y, left: state.x });
  // biome-ignore lint/correctness/useExhaustiveDependencies: state.items isn't read directly, but the menu's measured height depends on it, so the clamp must recompute when the item list changes (e.g. reopening at the same coordinates with a different number of items).
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const margin = 8;
    const maxLeft = window.innerWidth - el.offsetWidth - margin;
    const maxTop = window.innerHeight - el.offsetHeight - margin;
    setPos({
      left: Math.max(margin, Math.min(state.x, maxLeft)),
      top: Math.max(margin, Math.min(state.y, maxTop)),
    });
  }, [state.x, state.y, state.items]);

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
      style={{ top: pos.top, left: pos.left }}
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
