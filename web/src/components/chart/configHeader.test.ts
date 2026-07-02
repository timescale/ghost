import { describe, expect, test } from 'bun:test';

import {
  ASYNC_CONFIG_HEADER,
  CONFIG_HEADER,
  configHeaderFor,
  ensureChartTypeAnnotation,
} from './configHeader';

describe('configHeaderFor', () => {
  test('picks the sync header for a plain function declaration', () => {
    expect(configHeaderFor('function chart(data) { return {}; }')).toBe(
      CONFIG_HEADER,
    );
  });

  test('picks the sync header for a Promise-returning (non-async) config', () => {
    expect(
      configHeaderFor('function chart(data) { return Promise.resolve({}); }'),
    ).toBe(CONFIG_HEADER);
  });

  test('picks the async header for an async function declaration', () => {
    expect(configHeaderFor('async function chart(data) { return {}; }')).toBe(
      ASYNC_CONFIG_HEADER,
    );
  });

  test('picks the async header for an async arrow assignment', () => {
    expect(configHeaderFor('const chart = async (data) => ({});')).toBe(
      ASYNC_CONFIG_HEADER,
    );
  });

  test('picks the async header for an async function expression assignment', () => {
    expect(
      configHeaderFor('const chart = async function (data) { return {}; };'),
    ).toBe(ASYNC_CONFIG_HEADER);
  });

  test('ignores async helpers when chart itself is sync', () => {
    const config = `async function loadGeo() { return {}; }
function chart(data) { return {}; }`;
    expect(configHeaderFor(config)).toBe(CONFIG_HEADER);
  });

  test('headers are single-line (no newline shifts reported line numbers)', () => {
    expect(CONFIG_HEADER).not.toContain('\n');
    expect(ASYNC_CONFIG_HEADER).not.toContain('\n');
    // A trailing space separates the header from the config's first token.
    expect(CONFIG_HEADER.endsWith(' ')).toBe(true);
    expect(ASYNC_CONFIG_HEADER.endsWith(' ')).toBe(true);
  });
});

describe('ensureChartTypeAnnotation', () => {
  test('prepends the sync annotation line to a bare sync config', () => {
    expect(
      ensureChartTypeAnnotation('function chart(data) { return {}; }'),
    ).toBe('/** @type {ChartFunction} */\nfunction chart(data) { return {}; }');
  });

  test('prepends the async annotation line to a bare async config', () => {
    expect(
      ensureChartTypeAnnotation('async function chart(data) { return {}; }'),
    ).toBe(
      '/** @type {AsyncChartFunction} */\nasync function chart(data) { return {}; }',
    );
  });

  test('is idempotent (the added line is itself a leading JSDoc)', () => {
    const once = ensureChartTypeAnnotation('function chart() { return {}; }');
    expect(ensureChartTypeAnnotation(once)).toBe(once);
  });

  test('leaves a config with its own leading JSDoc untouched', () => {
    const config = `/**\n * @param {ChartData} data\n * @returns {EChartsOption}\n */\nfunction chart(data) { return {}; }`;
    expect(ensureChartTypeAnnotation(config)).toBe(config);
  });

  test('leaves a config with leading whitespace before its JSDoc untouched', () => {
    const config = '\n  /** docs */\nfunction chart() { return {}; }';
    expect(ensureChartTypeAnnotation(config)).toBe(config);
  });

  test('still annotates when a line comment leads the config', () => {
    // Line comments don't displace a preceding JSDoc annotation, so the
    // annotation is added and still binds to the function.
    expect(
      ensureChartTypeAnnotation('// my chart\nfunction chart() { return {}; }'),
    ).toBe(
      '/** @type {ChartFunction} */\n// my chart\nfunction chart() { return {}; }',
    );
  });
});
