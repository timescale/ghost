import { useEffect, useRef, useState } from 'react';

import { Icon } from './Icon';

// CopyButton copies the given text to the clipboard and briefly animates to a
// green checkmark for feedback. Rendered inside the QueryWidget toolbar.
export function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (resetTimer.current) clearTimeout(resetTimer.current);
    },
    [],
  );

  const onCopy = () => {
    void navigator.clipboard.writeText(text);
    setCopied(true);
    if (resetTimer.current) clearTimeout(resetTimer.current);
    resetTimer.current = setTimeout(() => setCopied(false), 1500);
  };

  return (
    <button
      type="button"
      onClick={onCopy}
      aria-label={copied ? 'Copied' : 'Copy to clipboard'}
      title={copied ? 'Copied' : 'Copy to clipboard'}
      className={`rounded border p-1.5 transition-colors ${
        copied
          ? 'border-green-300 bg-green-50 text-green-600'
          : 'border-slate-300 bg-white text-slate-600 hover:bg-slate-50 hover:text-slate-800'
      }`}
    >
      <span className="relative block size-4">
        <Icon
          name="copy"
          size={16}
          className={`absolute inset-0 transition-all duration-200 ${
            copied ? 'scale-50 opacity-0' : 'scale-100 opacity-100'
          }`}
        />
        <Icon
          name="check"
          size={16}
          className={`absolute inset-0 transition-all duration-200 ${
            copied ? 'scale-100 opacity-100' : 'scale-50 opacity-0'
          }`}
        />
      </span>
    </button>
  );
}
