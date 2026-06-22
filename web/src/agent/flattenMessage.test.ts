import { describe, expect, test } from 'bun:test';

import { flattenMessage } from './flattenMessage';

describe('flattenMessage', () => {
  test('returns a plain string unchanged', () => {
    expect(flattenMessage('simple error')).toBe('simple error');
  });

  test('returns the top message when there are no children', () => {
    expect(flattenMessage({ messageText: 'top' })).toBe('top');
  });

  test('indents one level of nested detail', () => {
    expect(
      flattenMessage({
        messageText: 'top',
        next: [{ messageText: 'child' }],
      }),
    ).toBe('top\n  child');
  });

  test('indents deeper chains cumulatively', () => {
    expect(
      flattenMessage({
        messageText: 'a',
        next: [{ messageText: 'b', next: [{ messageText: 'c' }] }],
      }),
    ).toBe('a\n  b\n    c');
  });

  test('handles multiple siblings', () => {
    expect(
      flattenMessage({
        messageText: 'top',
        next: [{ messageText: 'one' }, { messageText: 'two' }],
      }),
    ).toBe('top\n  one\n  two');
  });
});
