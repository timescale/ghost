import { describe, expect, test } from 'bun:test';

import { longestAlphanumericRun, trigrams, wordSimilarity } from './trigram';

describe('trigrams', () => {
  test('pads each word with two leading and one trailing space (pg_trgm)', () => {
    expect([...trigrams('word')]).toEqual(['  w', ' wo', 'wor', 'ord', 'rd ']);
  });

  test('lowercases input', () => {
    expect(trigrams('WORD')).toEqual(trigrams('word'));
  });

  test('splits on non-alphanumeric word boundaries', () => {
    expect([...trigrams('a_b')]).toEqual([...trigrams('a b')]);
  });

  test('is empty for input with no alphanumeric characters', () => {
    expect(trigrams('').size).toBe(0);
    expect(trigrams('__').size).toBe(0);
  });
});

describe('wordSimilarity', () => {
  test('is 1 when query trigrams are a subset of the name trigrams', () => {
    expect(wordSimilarity(trigrams('user'), trigrams('user'))).toBe(1);
  });

  test('is the fraction of query trigrams found in the name', () => {
    // 'usr' trigrams: '  u', ' us', 'usr', 'sr ' (4). 'user' shares '  u', ' us'.
    expect(wordSimilarity(trigrams('usr'), trigrams('user'))).toBeCloseTo(0.5);
  });

  test('is 0 when the query has no trigrams', () => {
    expect(wordSimilarity(trigrams(''), trigrams('user'))).toBe(0);
  });

  test('is asymmetric — normalized by the query, not the union', () => {
    // A short query fully contained in a long name scores high.
    const long = trigrams('user_sessions');
    expect(wordSimilarity(trigrams('user'), long)).toBe(1);
    expect(wordSimilarity(long, trigrams('user'))).toBeLessThan(1);
  });
});

describe('longestAlphanumericRun', () => {
  test('returns the longest run of alphanumeric characters', () => {
    expect(longestAlphanumericRun('abc.de')).toBe(3);
    expect(longestAlphanumericRun('a.b.c')).toBe(1);
    expect(longestAlphanumericRun('order_items')).toBe(5); // 'order' and 'items'
    expect(longestAlphanumericRun('')).toBe(0);
    expect(longestAlphanumericRun('___')).toBe(0);
  });
});
