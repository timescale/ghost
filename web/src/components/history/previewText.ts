// A one-line preview of a history entry's content (SQL or chart config) for the
// list: trimmed with all internal whitespace collapsed to single spaces. Shared
// by every history panel's left-column list.
export function previewText(text: string): string {
  return text.trim().replace(/\s+/g, ' ');
}
