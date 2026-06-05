// Mirror of internal/common/schema.go's JSON output. Field names match the
// snake_case JSON tags emitted by GET /api/schema.

export interface DatabaseSchema {
  id: string;
  name: string;
  schemas: NamespacedSchema[] | null;
}

export interface NamespacedSchema {
  name: string;
  tables?: TableSchema[];
  views?: ViewSchema[];
  materialized_views?: ViewSchema[];
  enums?: EnumSchema[];
  functions?: Routine[];
  procedures?: Routine[];
}

export interface TableSchema {
  name: string;
  columns?: TableColumn[];
  constraints?: TableConstraint[];
  indexes?: IndexSchema[];
  checks?: CheckConstraint[];
  exclusions?: ExclusionConstraint[];
  triggers?: TriggerSchema[];
  hypertable?: HypertableInfo;
}

export interface TableColumn {
  name: string;
  type: string;
  not_null?: boolean;
  default?: string;
  is_serial?: boolean;
  identity_type?: string;
}

export interface ViewSchema {
  name: string;
  columns?: ViewColumn[];
  // The view's defining SELECT (from pg_get_viewdef). Absent for tables.
  definition?: string;
  indexes?: IndexSchema[];
}

export interface ViewColumn {
  name: string;
  type: string;
}

export interface TableConstraint {
  type: 'PRIMARY KEY' | 'UNIQUE' | 'FOREIGN KEY';
  name: string;
  columns?: string[];
  ref_table?: string;
  ref_columns?: string[];
}

export interface IndexSchema {
  name: string;
  columns: string;
  definition: string;
  is_unique?: boolean;
  where_clause?: string;
}

export interface CheckConstraint {
  name: string;
  columns?: string[];
  expression: string;
}

export interface ExclusionConstraint {
  name: string;
  definition: string;
}

export interface EnumSchema {
  name: string;
  values?: string[];
}

export interface TriggerSchema {
  name: string;
  timing: string;
  manipulation: string;
  statement: string;
}

export interface Routine {
  name: string;
  // Identity argument list (e.g. "integer, text") that distinguishes
  // overloaded routines sharing a name. Absent for zero-argument routines.
  arguments?: string;
  type: 'FUNCTION' | 'PROCEDURE';
  definition?: string;
}

// routineSignature renders a routine's display label including its argument
// list, so overloaded routines that share a name are distinguishable and
// produce stable, unique React keys (e.g. "add(integer, integer)").
export function routineSignature(routine: Routine): string {
  return `${routine.name}(${routine.arguments ?? ''})`;
}

export interface HypertableInfo {
  compression_enabled: boolean;
  num_chunks: number;
}

// quoteIdent wraps a Postgres identifier with double-quotes, escaping any
// embedded quotes. Conservative: applied unconditionally so that names with
// uppercase letters or special characters round-trip safely.
export function quoteIdent(name: string): string {
  return `"${name.replace(/"/g, '""')}"`;
}

export function qualifiedName(schema: string, name: string): string {
  return `${quoteIdent(schema)}.${quoteIdent(name)}`;
}

export function selectAllSql(
  schema: string,
  name: string,
  columns?: { name: string }[],
): string {
  const cols =
    columns && columns.length > 0
      ? columns.map((c) => quoteIdent(c.name)).join(', ')
      : '*';
  return `SELECT ${cols} FROM ${qualifiedName(schema, name)} LIMIT 100;`;
}
