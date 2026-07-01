// a simple, trailing debounce implementation
export function debounce<Args extends unknown[]>(
  fn: (...args: Args) => void,
  wait: number,
) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  // The args of the most recent pending call, so flush() can invoke fn with
  // them immediately (rather than waiting out the timer).
  let pendingArgs: Args | undefined;
  const debounced = (...args: Args) => {
    pendingArgs = args;
    clearTimeout(timer);
    timer = setTimeout(() => {
      timer = undefined;
      pendingArgs = undefined;
      fn(...args);
    }, wait);
  };
  // Cancel a pending call without invoking fn.
  debounced.cancel = () => {
    clearTimeout(timer);
    timer = undefined;
    pendingArgs = undefined;
  };
  // Invoke a pending call immediately (with its latest args) instead of
  // waiting out the timer. No-op if nothing is pending.
  debounced.flush = () => {
    if (timer === undefined) return;
    clearTimeout(timer);
    timer = undefined;
    const args = pendingArgs as Args;
    pendingArgs = undefined;
    fn(...args);
  };
  return debounced;
}
