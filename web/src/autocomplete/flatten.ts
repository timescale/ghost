import type { SchemaFetchResponse } from '@timescale/popsql-query-widget-cdn';

import type {
  DatabaseSchema,
  NamespacedSchema,
  Routine,
  TableSchema,
  ViewSchema,
} from '../schema';

// flattenSchema converts the nested DatabaseSchema returned by GET /api/schema
// into a flat list of searchable objects shaped like the autocomplete library's
// SchemaFetchResponse. This is the in-memory equivalent of the rows popsql's
// SchemaService selects from Postgres: one entry per schema, table, view,
// column, trigger, and routine. The `database` field is set to the Postgres
// database name on every row so it matches the `database` the autocomplete
// library puts on its requests (it defaults to the current database name).
//
// Object kinds outside the autocomplete SchemaType union (enums, indexes,
// partitions, checks, exclusions) are intentionally skipped — they aren't
// completable identifiers in a query.
export function flattenSchema(schema: DatabaseSchema): SchemaFetchResponse[] {
  const database = schema.name;
  const out: SchemaFetchResponse[] = [];

  for (const ns of schema.schemas ?? []) {
    out.push({ type: 'schema', name: ns.name, database, schema: ns.name });

    for (const table of ns.tables ?? []) {
      pushTable(out, database, ns, table);
    }
    // Materialized views and continuous aggregates both surface under `views`
    // / `materialized_views`; all map to the autocomplete 'view' type.
    for (const view of ns.views ?? []) {
      pushView(out, database, ns, view);
    }
    for (const view of ns.materialized_views ?? []) {
      pushView(out, database, ns, view);
    }
    for (const routine of ns.functions ?? []) {
      pushRoutine(out, database, ns, routine);
    }
    for (const routine of ns.procedures ?? []) {
      pushRoutine(out, database, ns, routine);
    }
  }

  return out;
}

function pushTable(
  out: SchemaFetchResponse[],
  database: string,
  ns: NamespacedSchema,
  table: TableSchema,
): void {
  out.push({
    type: 'table',
    name: table.name,
    database,
    schema: ns.name,
    comment: table.comment ?? null,
  });

  // Derive primary-key / unique flags from the table's constraints so column
  // suggestions carry the same metadata popsql shows.
  const primaryKeyColumns = new Set<string>();
  const uniqueColumns = new Set<string>();
  for (const constraint of table.constraints ?? []) {
    const target =
      constraint.type === 'PRIMARY KEY'
        ? primaryKeyColumns
        : constraint.type === 'UNIQUE'
          ? uniqueColumns
          : null;
    if (target) {
      for (const column of constraint.columns ?? []) target.add(column);
    }
  }

  for (const column of table.columns ?? []) {
    out.push({
      type: 'column',
      name: column.name,
      database,
      schema: ns.name,
      table: table.name,
      dataType: column.type,
      isNotNull: column.not_null ?? null,
      defaultValue: column.default ?? null,
      isPrimaryKey: primaryKeyColumns.has(column.name),
      isUniqueKey: uniqueColumns.has(column.name),
      comment: column.comment ?? null,
    });
  }

  pushTriggers(out, database, ns, table.name, table.triggers);
}

function pushView(
  out: SchemaFetchResponse[],
  database: string,
  ns: NamespacedSchema,
  view: ViewSchema,
): void {
  out.push({
    type: 'view',
    name: view.name,
    database,
    schema: ns.name,
    comment: view.comment ?? null,
    definition: view.definition ?? null,
  });

  for (const column of view.columns ?? []) {
    out.push({
      type: 'column',
      name: column.name,
      database,
      schema: ns.name,
      table: view.name,
      dataType: column.type,
      comment: column.comment ?? null,
    });
  }

  pushTriggers(out, database, ns, view.name, view.triggers);
}

function pushTriggers(
  out: SchemaFetchResponse[],
  database: string,
  ns: NamespacedSchema,
  table: string,
  triggers: TableSchema['triggers'],
): void {
  for (const trigger of triggers ?? []) {
    out.push({
      type: 'trigger',
      name: trigger.name,
      database,
      schema: ns.name,
      table,
      timing: trigger.timing,
      manipulation: trigger.manipulation,
      statement: trigger.statement,
    });
  }
}

function pushRoutine(
  out: SchemaFetchResponse[],
  database: string,
  ns: NamespacedSchema,
  routine: Routine,
): void {
  out.push({
    type: 'routine',
    name: routine.name,
    database,
    schema: ns.name,
    routineType: routine.type,
    definition: routine.definition ?? null,
    comment: routine.comment ?? null,
  });
}
