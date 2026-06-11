// Mirror of internal/common/schema.go's JSON output. Field names match the
// snake_case JSON tags emitted by GET /api/schema.

export interface DatabaseSchema {
  id: string;
  name: string;
  schemas: NamespacedSchema[] | null;
}

export interface NamespacedSchema {
  name: string;
  // COMMENT ON SCHEMA text. Absent unless comments were requested (the Go
  // field is `comment,omitempty`); the schema pane always requests them.
  comment?: string;
  tables?: TableSchema[];
  views?: ViewSchema[];
  materialized_views?: ViewSchema[];
  enums?: EnumSchema[];
  functions?: Routine[];
  procedures?: Routine[];
}

export interface TableSchema {
  name: string;
  // COMMENT ON TABLE text. Absent unless comments were requested.
  comment?: string;
  columns?: TableColumn[];
  constraints?: TableConstraint[];
  indexes?: IndexSchema[];
  checks?: CheckConstraint[];
  exclusions?: ExclusionConstraint[];
  triggers?: TriggerSchema[];
  // Child partitions of a partitioned table. Only present for partitioned
  // tables; the children are hidden as standalone tables.
  partitions?: PartitionInfo[];
  hypertable?: HypertableInfo;
  // FDW binding of a foreign table (relkind 'f'). Absent for regular
  // tables. Foreign tables are modeled as tables; this field is what
  // distinguishes them.
  foreign?: ForeignTableInfo;
}

export interface PartitionInfo {
  name: string;
  // The partition child's schema. Only present when the partition lives in a
  // different schema than its parent table (PostgreSQL allows this). When
  // absent, the partition shares its parent's schema.
  schema?: string;
  // The partition's bound expression, e.g.
  // "FOR VALUES FROM ('2024-01-01') TO ('2025-01-01')".
  bound?: string;
}

export interface TableColumn {
  name: string;
  type: string;
  // COMMENT ON COLUMN text. Absent unless comments were requested.
  comment?: string;
  not_null?: boolean;
  default?: string;
  is_serial?: boolean;
  identity_type?: string;
}

export interface ViewSchema {
  name: string;
  // COMMENT ON (MATERIALIZED) VIEW text. Absent unless comments were
  // requested.
  comment?: string;
  columns?: ViewColumn[];
  // The view's defining SELECT (from pg_get_viewdef). Absent for tables.
  definition?: string;
  indexes?: IndexSchema[];
  // Triggers defined on the view (e.g. INSTEAD OF triggers). Not
  // applicable to materialized views.
  triggers?: TriggerSchema[];
  // TimescaleDB continuous aggregate metadata. Absent for ordinary views.
  // A continuous aggregate is a regular view over an internal
  // materialization hypertable, so it appears under `views`; this field is
  // what distinguishes it. When present, `definition` holds the user's
  // original defining query rather than the rewritten SELECT over the
  // internal materialization hypertable.
  continuous_aggregate?: ContinuousAggregateInfo;
}

export interface ViewColumn {
  name: string;
  type: string;
  // COMMENT ON COLUMN text. Absent unless comments were requested.
  comment?: string;
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
  // The CREATE INDEX statement (from pg_get_indexdef). Absent unless
  // definitions were requested (the Go field is `definition,omitempty`).
  definition?: string;
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
  // COMMENT ON TYPE text. Absent unless comments were requested.
  comment?: string;
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
  // COMMENT ON FUNCTION/PROCEDURE text. Absent unless comments were
  // requested.
  comment?: string;
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

// hypertableDetails renders hypertable metadata as readable plain text, used
// by both the hypertable pill tooltip and the "View hypertable details"
// modal.
export function hypertableDetails(info: HypertableInfo): string {
  return [
    `Chunks:      ${info.num_chunks}`,
    `Compression: ${info.compression_enabled ? 'enabled' : 'disabled'}`,
  ].join('\n');
}

export interface ContinuousAggregateInfo {
  compression_enabled: boolean;
  // Whether queries against the view return only already-materialized data
  // (true) or also combine the not-yet-materialized recent data in real
  // time (false).
  materialized_only: boolean;
}

// continuousAggregateDetails renders continuous aggregate metadata as
// readable plain text, used by both the cagg pill tooltip and the "View
// continuous aggregate details" modal.
export function continuousAggregateDetails(
  info: ContinuousAggregateInfo,
): string {
  return [
    `Materialized only: ${info.materialized_only ? 'yes' : 'no'}`,
    `Compression:       ${info.compression_enabled ? 'enabled' : 'disabled'}`,
  ].join('\n');
}

export interface ForeignTableInfo {
  // pg_foreign_server.srvname
  server: string;
  // pg_foreign_data_wrapper.fdwname
  wrapper: string;
  // Table-level ftoptions as "key=value" strings (e.g. "table_name=orders").
  // Server-level options and user mappings are never exposed.
  options?: string[];
}

// foreignTableDetails renders an FDW binding as readable plain text, used by
// both the FDW pill tooltip and the "View FDW details" modal.
export function foreignTableDetails(info: ForeignTableInfo): string {
  const lines = [`Server:  ${info.server}`, `Wrapper: ${info.wrapper}`];
  if (info.options && info.options.length > 0) {
    lines.push('Options:', ...info.options.map((opt) => `  ${opt}`));
  }
  return lines.join('\n');
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
