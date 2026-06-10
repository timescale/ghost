// copyText writes the given text to the system clipboard. Fire-and-forget:
// clipboard writes only fail in exotic cases (permissions, non-secure
// contexts) where there is nothing actionable to surface to the user.
export function copyText(text: string) {
  void navigator.clipboard.writeText(text).catch(console.error);
}
