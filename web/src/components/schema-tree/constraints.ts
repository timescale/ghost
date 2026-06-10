import type {
  TableColumn,
  TableConstraint,
  TableSchema,
  ViewSchema,
} from '../../schema';

// A single row under the table's Constraints group.
export interface ConstraintItem {
  name: string;
  kindWord: string;
  detail: string;
}

// tableConstraintItems flattens the constraints a table carries that aren't
// already conveyed by the per-column pills: composite primary-key/unique
// constraints, foreign keys (with their full referenced table/columns),
// check constraints, and exclusion constraints. Single-column PK/UNIQUE
// membership is omitted here because it's shown inline on each member
// column (see columnConstraintLabel).
export function tableConstraintItems(table: TableSchema): ConstraintItem[] {
  const items: ConstraintItem[] = [];
  for (const c of table.constraints ?? []) {
    const cols = c.columns ?? [];
    if (c.type === 'PRIMARY KEY' || c.type === 'UNIQUE') {
      // Single-column PK/UNIQUE are already shown inline on the column.
      if (cols.length <= 1) continue;
      items.push({
        name: c.name,
        kindWord: c.type === 'PRIMARY KEY' ? 'primary key' : 'unique',
        detail: `(${cols.join(', ')})`,
      });
      continue;
    }
    if (c.type !== 'FOREIGN KEY') continue;
    const colsList = cols.join(', ');
    const refCols = (c.ref_columns ?? []).join(', ');
    items.push({
      name: c.name,
      kindWord: 'foreign key',
      detail: `(${colsList}) \u2192 ${c.ref_table ?? '?'}(${refCols})`,
    });
  }
  for (const chk of table.checks ?? []) {
    items.push({ name: chk.name, kindWord: 'check', detail: chk.expression });
  }
  for (const exc of table.exclusions ?? []) {
    items.push({ name: exc.name, kindWord: 'exclude', detail: exc.definition });
  }
  return items;
}

// columnConstraintLabel picks the single most informative constraint label
// for a column, in priority order: primary key > unique > not null.
// Mirrors popsql's `constraintForColumn`.
export function columnConstraintLabel(
  parent: TableSchema | ViewSchema,
  col: TableColumn | { name: string },
): string | null {
  const t = parent as TableSchema;
  const constraints = t.constraints ?? [];
  // Only single-column PK/UNIQUE constraints are conveyed inline on the
  // column. Composite (multi-column) constraints would be misleading as a
  // per-column pill (e.g. UNIQUE (a, b) does not make `a` unique on its
  // own), so those are surfaced under the table's Constraints group
  // instead (see tableConstraintItems).
  const hasSingleColumn = (type: TableConstraint['type']) =>
    constraints.some(
      (c) =>
        c.type === type &&
        (c.columns ?? []).length === 1 &&
        c.columns?.[0] === col.name,
    );
  if (hasSingleColumn('PRIMARY KEY')) {
    return 'primary key';
  }
  if (hasSingleColumn('UNIQUE')) {
    return 'unique';
  }
  if ((col as TableColumn).not_null) {
    return 'not null';
  }
  return null;
}

// columnForeignKey returns the referenced table for a single-column foreign
// key on the given column, or null. Surfaced inline as a hint pill on the
// column row; the full constraint (with referenced columns) is also listed
// under the table's Constraints group.
export function columnForeignKey(
  parent: TableSchema | ViewSchema,
  col: { name: string },
): string | null {
  const constraints = (parent as TableSchema).constraints ?? [];
  for (const c of constraints) {
    const cols = c.columns ?? [];
    if (
      c.type === 'FOREIGN KEY' &&
      cols.length === 1 &&
      cols[0] === col.name &&
      c.ref_table
    ) {
      return c.ref_table;
    }
  }
  return null;
}
