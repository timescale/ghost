import { describe, expect, test } from 'bun:test';

import { rowsToMatrix } from './runData';

describe('rowsToMatrix', () => {
  test('projects row objects into positional arrays aligned to columns', () => {
    const rows = [
      { time: '2024-01-01', temp: 21.5, humidity: 40 },
      { time: '2024-01-02', temp: 22.0, humidity: 38 },
    ];
    const columns = [{ name: 'time' }, { name: 'temp' }, { name: 'humidity' }];
    expect(rowsToMatrix(rows, columns)).toEqual([
      ['2024-01-01', 21.5, 40],
      ['2024-01-02', 22.0, 38],
    ]);
  });

  test('respects column order, not row key order', () => {
    const rows = [{ b: 2, a: 1 }];
    const columns = [{ name: 'a' }, { name: 'b' }];
    expect(rowsToMatrix(rows, columns)).toEqual([[1, 2]]);
  });

  test('maps missing/undefined values to null', () => {
    const rows = [{ a: 1 }];
    const columns = [{ name: 'a' }, { name: 'missing' }];
    expect(rowsToMatrix(rows, columns)).toEqual([[1, null]]);
  });
});
