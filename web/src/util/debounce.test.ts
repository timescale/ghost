import { describe, expect, test } from 'bun:test';

import { debounce } from './debounce';

const tick = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

describe('debounce', () => {
  test('invokes the fn once after the wait, with the latest args', async () => {
    const calls: number[] = [];
    const d = debounce((n: number) => calls.push(n), 20);
    d(1);
    d(2);
    d(3);
    expect(calls).toEqual([]);
    await tick(30);
    expect(calls).toEqual([3]);
  });

  test('cancel() drops a pending call without invoking fn', async () => {
    const calls: number[] = [];
    const d = debounce((n: number) => calls.push(n), 20);
    d(1);
    d.cancel();
    await tick(30);
    expect(calls).toEqual([]);
  });

  test('flush() invokes a pending call immediately with its latest args', () => {
    const calls: number[] = [];
    const d = debounce((n: number) => calls.push(n), 20);
    d(1);
    d(2);
    d.flush();
    expect(calls).toEqual([2]);
  });

  test('flush() is a no-op when nothing is pending', () => {
    const calls: number[] = [];
    const d = debounce((n: number) => calls.push(n), 20);
    d.flush();
    expect(calls).toEqual([]);
  });

  test('flush() prevents the pending timer from firing again', async () => {
    const calls: number[] = [];
    const d = debounce((n: number) => calls.push(n), 20);
    d(1);
    d.flush();
    await tick(30);
    expect(calls).toEqual([1]);
  });
});
