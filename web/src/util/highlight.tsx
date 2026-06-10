import type { ReactNode } from 'react';

// highlight wraps the first case-insensitive occurrence of `term` in `text`
// with a <mark>, for emphasizing search matches in rendered labels. Returns
// the text unchanged when the term is empty or not found.
export function highlight(text: string, term: string): ReactNode {
  if (!term) return text;
  const idx = text.toLowerCase().indexOf(term.toLowerCase());
  if (idx < 0) return text;
  const before = text.slice(0, idx);
  const match = text.slice(idx, idx + term.length);
  const after = text.slice(idx + term.length);
  return (
    <>
      {before}
      <mark className="rounded bg-yellow-200 px-0.5">{match}</mark>
      {after}
    </>
  );
}
