import { type MouseEvent, type ReactNode, useEffect, useRef } from 'react';

interface Props {
  children: ReactNode;
  className?: string;
  onClose: () => void;
}

export function Modal({ children, className, onClose }: Props) {
  const ref = useRef<HTMLDivElement | null>(null);
  const downTarget = useRef<EventTarget | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  // Close on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCloseRef.current();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  return (
    // biome-ignore lint/a11y/useKeyWithClickEvents: the backdrop's onClick is a mouse-only click-outside affordance; the keyboard equivalent (Escape) is handled by the global window keydown listener above
    <div
      ref={ref}
      onClick={(e: MouseEvent<HTMLDivElement>) => {
        // Only close on click-outside when the mousedown also originated on the
        // backdrop, so dragging to select text inside the modal doesn't dismiss it.
        if (e.target === ref.current && downTarget.current === ref.current) {
          onClose();
        }
      }}
      onMouseDown={(e) => {
        downTarget.current = e.target;
      }}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/30"
    >
      <div
        className={`flex max-h-[80vh] max-w-[90vw] min-h-24 min-w-40 flex-col rounded-lg border border-slate-200 bg-white shadow-xl ${className ?? ''}`}
      >
        {children}
      </div>
    </div>
  );
}
