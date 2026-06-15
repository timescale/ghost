import { describe, expect, test } from 'bun:test';

import type { DatabaseSchema } from '../schema';
import { flattenSchema } from './flatten';

const schema: DatabaseSchema = {
  id: 'svc-1',
  name: 'tsdb',
  schemas: [
    {
      name: 'public',
      tables: [
        {
          name: 'users',
          comment: 'app users',
          columns: [
            { name: 'id', type: 'integer', not_null: true },
            { name: 'email', type: 'text' },
          ],
          constraints: [
            { type: 'PRIMARY KEY', name: 'users_pkey', columns: ['id'] },
            { type: 'UNIQUE', name: 'users_email_key', columns: ['email'] },
          ],
          triggers: [
            {
              name: 'users_audit',
              timing: 'AFTER',
              manipulation: 'UPDATE',
              statement: 'EXECUTE FUNCTION audit()',
            },
          ],
        },
      ],
      views: [
        {
          name: 'active_users',
          definition: 'SELECT id FROM users',
          columns: [{ name: 'id', type: 'integer' }],
        },
      ],
      functions: [
        { name: 'now2', type: 'FUNCTION', definition: 'SELECT now()' },
      ],
      enums: [{ name: 'mood', values: ['happy', 'sad'] }],
    },
  ],
};

describe('flattenSchema', () => {
  const rows = flattenSchema(schema);
  const byKey = (type: string, name: string) =>
    rows.find((r) => r.type === type && r.name === name);

  test('emits a schema row', () => {
    expect(byKey('schema', 'public')).toEqual({
      type: 'schema',
      name: 'public',
      database: 'tsdb',
      schema: 'public',
    });
  });

  test('emits a table row with its comment', () => {
    expect(byKey('table', 'users')).toMatchObject({
      type: 'table',
      name: 'users',
      database: 'tsdb',
      schema: 'public',
      comment: 'app users',
    });
  });

  test('derives primary-key and unique flags on columns from constraints', () => {
    expect(byKey('column', 'id')).toMatchObject({
      type: 'column',
      table: 'users',
      dataType: 'integer',
      isNotNull: true,
      isPrimaryKey: true,
      isUniqueKey: false,
    });
    expect(byKey('column', 'email')).toMatchObject({
      isPrimaryKey: false,
      isUniqueKey: true,
    });
  });

  test('emits view rows, view columns, triggers, and routines', () => {
    expect(byKey('view', 'active_users')).toMatchObject({
      definition: 'SELECT id FROM users',
    });
    // The view's column shares the column name with the table's; both exist,
    // distinguished by their `table` owner.
    expect(
      rows.filter((r) => r.type === 'column' && r.name === 'id'),
    ).toHaveLength(2);
    expect(byKey('trigger', 'users_audit')).toMatchObject({
      table: 'users',
      timing: 'AFTER',
      manipulation: 'UPDATE',
    });
    expect(byKey('routine', 'now2')).toMatchObject({
      routineType: 'FUNCTION',
      definition: 'SELECT now()',
    });
  });

  test('skips object kinds outside the autocomplete type union (enums)', () => {
    expect(rows.some((r) => r.name === 'mood')).toBe(false);
  });
});
