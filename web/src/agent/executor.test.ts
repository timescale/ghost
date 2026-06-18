import { beforeEach, describe, expect, test } from 'bun:test';

import {
  awaitExecutor,
  type Executor,
  getExecutor,
  registerExecutor,
} from './executor';

function makeExecutor(databaseId: string): Executor {
  return {
    databaseId,
    runQuery: async () => ({ runId: 'r', status: 'success' as const }),
    getRunData: async () => ({ rows: [], columns: [] }),
  };
}

describe('executor registry', () => {
  beforeEach(() => {
    // Clear any executor left registered by a prior test.
    const cleanup = registerExecutor(makeExecutor('reset'));
    cleanup();
  });

  test('getExecutor returns the registered executor', () => {
    const exec = makeExecutor('db1');
    registerExecutor(exec);
    expect(getExecutor()).toBe(exec);
  });

  test('unregister clears only the matching executor', () => {
    const exec1 = makeExecutor('db1');
    const cleanup1 = registerExecutor(exec1);
    const exec2 = makeExecutor('db2');
    registerExecutor(exec2);
    // exec1's cleanup should not clear exec2 (the current one).
    cleanup1();
    expect(getExecutor()).toBe(exec2);
  });

  test('awaitExecutor resolves immediately for the mounted database', async () => {
    const exec = makeExecutor('db1');
    registerExecutor(exec);
    await expect(awaitExecutor('db1', 1000)).resolves.toBe(exec);
  });

  test('awaitExecutor waits for the matching database to register', async () => {
    const promise = awaitExecutor('db2', 1000);
    // A non-matching executor mounts first; should keep waiting.
    registerExecutor(makeExecutor('db1'));
    const exec2 = makeExecutor('db2');
    registerExecutor(exec2);
    await expect(promise).resolves.toBe(exec2);
  });

  test('awaitExecutor rejects on timeout', async () => {
    await expect(awaitExecutor('never', 10)).rejects.toThrow(
      'timed out waiting for the database panel to load',
    );
  });
});
