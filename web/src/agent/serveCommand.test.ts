import { describe, expect, test } from 'bun:test';

import { serveCommand } from './serveCommand';

describe('serveCommand', () => {
  test('pins the current port for a loopback host (no --host)', () => {
    expect(serveCommand({ hostname: '127.0.0.1', port: '5174' })).toBe(
      'ghost serve -n -p 5174',
    );
    expect(serveCommand({ hostname: 'localhost', port: '8080' })).toBe(
      'ghost serve -n -p 8080',
    );
    expect(serveCommand({ hostname: '::1', port: '3000' })).toBe(
      'ghost serve -n -p 3000',
    );
  });

  test('includes --host for a non-loopback host', () => {
    expect(serveCommand({ hostname: '192.168.1.50', port: '5174' })).toBe(
      'ghost serve -n --host 192.168.1.50 -p 5174',
    );
    expect(serveCommand({ hostname: 'my-box.local', port: '5174' })).toBe(
      'ghost serve -n --host my-box.local -p 5174',
    );
  });

  test('falls back to port 80 when the port is empty', () => {
    expect(serveCommand({ hostname: '127.0.0.1', port: '' })).toBe(
      'ghost serve -n -p 80',
    );
  });
});
