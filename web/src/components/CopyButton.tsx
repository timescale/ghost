import { useEffect, useRef, useState } from 'react';

import { Icon } from './Icon';

type CopyState = 'idle' | 'copied' | 'error';

// CopyButton copies the given text to the clipboard and briefly animates to a
// green checkmark for feedback, or a red x if the write fails (e.g. denied
// permissions or a non-secure context). Rendered inside the QueryWidget
// toolbar.
export function CopyButton({ text }: { text: string }) {
  const [state, setState] = useState<CopyState>('idle');
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (resetTimer.current) clearTimeout(resetTimer.current);
    },
    [],
  );

  const onCopy = (): void => {
    (async () => {
      try {
        await navigator.clipboard.writeText(text);
        setState('copied');
      } catch (err) {
        console.error('failed to copy to clipboard', err);
        setState('error');
      }
      if (resetTimer.current) clearTimeout(resetTimer.current);
      resetTimer.current = setTimeout(() => setState('idle'), 1500);
    })().catch(console.error);
  };

  const copied = state === 'copied';
  const error = state === 'error';

  return (
    <button
      type="button"
      onClick={onCopy}
      aria-label={
        error ? 'Copy failed' : copied ? 'Copied' : 'Copy to clipboard'
      }
      title={error ? 'Copy failed' : copied ? 'Copied' : 'Copy to clipboard'}
      className={`rounded border p-1.5 transition-colors ${
        error
          ? 'border-red-300 bg-red-50 text-red-600'
          : copied
            ? 'border-green-300 bg-green-50 text-green-600'
            : 'border-slate-300 bg-white text-slate-600 hover:bg-slate-50 hover:text-slate-800'
      }`}
    >
      <span className="relative block size-4">
        <Icon
          name="copy"
          size={16}
          className={`absolute inset-0 transition-all duration-200 ${
            state === 'idle' ? 'scale-100 opacity-100' : 'scale-50 opacity-0'
          }`}
        />
        <Icon
          name="check"
          size={16}
          className={`absolute inset-0 transition-all duration-200 ${
            copied ? 'scale-100 opacity-100' : 'scale-50 opacity-0'
          }`}
        />
        <Icon
          name="x"
          size={16}
          className={`absolute inset-0 transition-all duration-200 ${
            error ? 'scale-100 opacity-100' : 'scale-50 opacity-0'
          }`}
        />
      </span>
    </button>
  );
}
