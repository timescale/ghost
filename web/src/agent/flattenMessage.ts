// A TypeScript diagnostic message can be a plain string or a nested chain of
// related messages (e.g. "Type X is not assignable to Y" with sub-reasons).
// This mirrors the shape Monaco's TS worker returns.
export interface DiagnosticMessageChain {
  messageText: string;
  next?: DiagnosticMessageChain[];
}

// flattenMessage flattens a (possibly nested) diagnostic message chain into a
// single string, indenting nested detail so the structure stays legible to the
// agent reading it as text.
export function flattenMessage(
  messageText: string | DiagnosticMessageChain,
): string {
  if (typeof messageText === 'string') return messageText;
  let result = messageText.messageText;
  for (const child of messageText.next ?? []) {
    result += `\n  ${flattenMessage(child).replace(/\n/g, '\n  ')}`;
  }
  return result;
}
