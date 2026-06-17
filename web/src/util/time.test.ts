import { describe, expect, test } from 'bun:test';

import { formatRelativeTime } from './time';

describe('formatRelativeTime', () => {
  const now = 1_000_000_000_000;
  const sec = 1000;
  const min = 60 * sec;
  const hour = 60 * min;
  const day = 24 * hour;

  test('renders "just now" for very recent timestamps', () => {
    expect(formatRelativeTime(now, now)).toBe('just now');
    expect(formatRelativeTime(now - 30 * sec, now)).toBe('just now');
  });

  test('renders minutes, hours, and days', () => {
    expect(formatRelativeTime(now - 5 * min, now)).toBe('5m ago');
    expect(formatRelativeTime(now - 3 * hour, now)).toBe('3h ago');
    expect(formatRelativeTime(now - 2 * day, now)).toBe('2d ago');
  });

  test('renders weeks, months, and years', () => {
    expect(formatRelativeTime(now - 14 * day, now)).toBe('2w ago');
    expect(formatRelativeTime(now - 60 * day, now)).toBe('2mo ago');
    expect(formatRelativeTime(now - 800 * day, now)).toBe('2y ago');
  });

  test('clamps future timestamps to "just now"', () => {
    expect(formatRelativeTime(now + 5 * min, now)).toBe('just now');
  });
});
