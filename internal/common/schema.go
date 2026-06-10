package common

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/util"
)

// DatabaseSchema holds complete schema information for a database, grouped
// by namespace (Postgres schema).
type DatabaseSchema struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Schemas []NamespacedSchema `json:"schemas"`
}

// NamespacedSchema groups the objects belonging to a single Postgres schema.
type NamespacedSchema struct {
	Name              string        `json:"name"`
	Tables            []TableSchema `json:"tables,omitempty"`
	Views             []ViewSchema  `json:"views,omitempty"`
	MaterializedViews []ViewSchema  `json:"materialized_views,omitempty"`
	Enums             []EnumSchema  `json:"enums,omitempty"`
	Functions         []Routine     `json:"functions,omitempty"`
	Procedures        []Routine     `json:"procedures,omitempty"`
}

// TableSchema holds schema information for a table.
type TableSchema struct {
	Name        string                `json:"name"`
	Columns     []TableColumnSchema   `json:"columns,omitempty"`
	Constraints []TableConstraint     `json:"constraints,omitempty"`
	Indexes     []IndexSchema         `json:"indexes,omitempty"`
	Checks      []CheckConstraint     `json:"checks,omitempty"`
	Exclusions  []ExclusionConstraint `json:"exclusions,omitempty"`
	Triggers    []TriggerSchema       `json:"triggers,omitempty"`
	// Partitions lists the direct child partitions of a partitioned table.
	// Only populated for partitioned tables (relkind 'p'). Leaf partitions
	// are normally hidden as standalone tables, but in a multi-level hierarchy
	// an intermediate partitioned table is shown both as an entry here (under
	// its parent) and as its own table carrying its sub-partitions. When a
	// single schema is requested, a leaf whose parent lives in a different
	// schema is shown as a standalone table instead (see leafPartitionExclusion).
	Partitions []PartitionInfo `json:"partitions,omitempty"`
	Hypertable *HypertableInfo `json:"hypertable,omitempty"`
}

// PartitionInfo describes a single child partition of a partitioned table.
type PartitionInfo struct {
	Name string `json:"name"`
	// Schema is the partition child's schema. It is only populated when the
	// partition lives in a different schema than its parent table (PostgreSQL
	// allows this), so that callers can schema-qualify the partition
	// correctly. When empty, the partition shares its parent's schema.
	Schema string `json:"schema,omitempty"`
	// Bound is the partition's bound expression (from pg_get_expr on
	// relpartbound), e.g. "FOR VALUES FROM ('2024-01-01') TO ('2025-01-01')".
	Bound string `json:"bound,omitempty"`
}

// ViewSchema holds schema information for a view or materialized view.
type ViewSchema struct {
	Name    string             `json:"name"`
	Columns []ViewColumnSchema `json:"columns,omitempty"`
	// Definition is the view's defining SELECT (from pg_get_viewdef).
	Definition string `json:"definition,omitempty"`
	// Indexes are only populated for materialized views.
	Indexes []IndexSchema `json:"indexes,omitempty"`
	// Triggers lists triggers defined on the view (e.g. INSTEAD OF
	// triggers on a regular view). Not applicable to materialized views.
	Triggers []TriggerSchema `json:"triggers,omitempty"`
}

// ViewColumnSchema holds column info for views (simpler than table columns).
type ViewColumnSchema struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// TableColumnSchema holds schema information for a table column.
type TableColumnSchema struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	NotNull      bool   `json:"not_null,omitempty"`
	Default      string `json:"default,omitempty"`
	IsSerial     bool   `json:"is_serial,omitempty"`
	IdentityType string `json:"identity_type,omitempty"`
}

// TableConstraint describes a constraint (single or multi-column).
type TableConstraint struct {
	Type       ConstraintType `json:"type"`
	Name       string         `json:"name"`
	Columns    []string       `json:"columns,omitempty"`
	RefTable   string         `json:"ref_table,omitempty"`
	RefColumns []string       `json:"ref_columns,omitempty"`
}

// ConstraintType represents the type of a table constraint.
type ConstraintType string

const (
	ConstraintPrimaryKey ConstraintType = "PRIMARY KEY"
	ConstraintUnique     ConstraintType = "UNIQUE"
	ConstraintForeignKey ConstraintType = "FOREIGN KEY"
)

// IndexSchema describes an index.
type IndexSchema struct {
	Name        string `json:"name"`
	Columns     string `json:"columns"`
	Definition  string `json:"definition,omitempty"`
	IsUnique    bool   `json:"is_unique,omitempty"`
	WhereClause string `json:"where_clause,omitempty"`
}

// CheckConstraint describes a check constraint.
type CheckConstraint struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns,omitempty"`
	Expression string   `json:"expression"`
}

// ExclusionConstraint describes an exclusion constraint.
type ExclusionConstraint struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

// EnumSchema describes an enum type.
type EnumSchema struct {
	Name   string   `json:"name"`
	Values []string `json:"values,omitempty"`
}

// TriggerSchema describes a single trigger on a table.
type TriggerSchema struct {
	Name         string `json:"name"`
	Timing       string `json:"timing"`
	Manipulation string `json:"manipulation"`
	Statement    string `json:"statement"`
}

// RoutineType is the type of a routine.
type RoutineType string

const (
	RoutineFunction  RoutineType = "FUNCTION"
	RoutineProcedure RoutineType = "PROCEDURE"
)

// Routine describes a function or procedure.
type Routine struct {
	Name string `json:"name"`
	// Arguments is the identity argument list (e.g. "integer, text"),
	// which distinguishes overloaded routines that share a name. Empty for
	// a routine that takes no arguments.
	Arguments  string      `json:"arguments,omitempty"`
	Type       RoutineType `json:"type"`
	Definition string      `json:"definition,omitempty"`
}

// HypertableInfo describes TimescaleDB hypertable metadata for a table.
type HypertableInfo struct {
	CompressionEnabled bool `json:"compression_enabled"`
	NumChunks          int  `json:"num_chunks"`
}

// FetchDatabaseSchemaArgs are the arguments to FetchDatabaseSchema.
type FetchDatabaseSchemaArgs struct {
	Client      api.ClientWithResponsesInterface
	ProjectID   string
	DatabaseRef string
	// Schema, if non-empty, limits the fetch to a single namespace.
	Schema string
	// IncludeInternal, when true, disables all schema/object exclusion
	// filters. Catalog (pg_*) and extension-owned objects will be included.
	IncludeInternal bool
	// IncludeDefinitions, when true, fetches full object definitions (view
	// SELECT statements and function/procedure bodies). These are omitted by
	// default because they can be large and may embed implementation details
	// or secrets; only fetch them when the caller will actually use them.
	IncludeDefinitions bool
}

// schemaFilter holds the SQL fragments needed to scope a query to the
// user-visible schemas / objects.
type schemaFilter struct {
	includeInternal    bool
	includeDefinitions bool
	schema             string
}

// definitionExpr returns the SQL expression to select for an object
// definition column (e.g. a view's defining SELECT or a routine's body).
// When definitions are not requested it returns NULL, so the heavy
// pg_get_*def catalog calls are skipped and definition text (which may
// embed implementation details or secrets) is never returned.
func (f schemaFilter) definitionExpr(expr string) string {
	if f.includeDefinitions {
		return expr
	}
	return "NULL"
}

// leafPartitionExclusion returns the WHERE clause that hides leaf partitions
// (partition children that aren't themselves partitioned tables) as
// standalone relations. Leaf partitions are normally surfaced under their
// parent's Partitions list instead, so showing them as their own tables
// would be redundant.
//
// That surfacing only works when the parent table is in scope to carry the
// child (i.e. the parent passes every filter and lands in tableIndex). A leaf
// whose parent is filtered out would otherwise vanish entirely: the parent is
// absent so nothing surfaces the child, and the child itself is suppressed
// here. A parent can be filtered out for several reasons:
//
//   - it lives in a different schema than the leaf (PostgreSQL allows this),
//     and that schema is excluded — either by an explicit --schema request or
//     by the default-browse name exclusions (pg_*, information_schema,
//     timescaledb internals, toolkit_experimental);
//   - it is extension-owned, inaccessible to the current user, or
//     superuser-owned (the same onExtensionObject/onAccessible/onUserOwned
//     filters the relations query applies to the leaf).
//
// To keep such a leaf visible, suppress a leaf only when its immediate parent
// would itself pass the relations query's filters. The EXISTS subquery below
// applies exactly those filters to the parent, so the predicate matches the
// condition under which fetchPartitions can attach the leaf to its parent
// (parent in tableIndex). Every leaf is therefore shown exactly once: grouped
// under its parent when the parent is in scope, or standalone otherwise.
//
// The relation's pg_class row is referenced via the relAlias argument so
// this clause can be spliced into any query regardless of how it aliases
// pg_class (e.g. "c" in the relations query, "t" in the indexes query).
// Both query builders must use the same predicate so a leaf that is surfaced
// as a standalone table also has its indexes listed.
//
// When a single schema is requested the parent's schema is referenced as $1,
// the same parameter onSchema binds; PostgreSQL allows a positional parameter
// to appear multiple times.
func (f schemaFilter) leafPartitionExclusion(relAlias string) string {
	return fmt.Sprintf(` AND NOT (
        %[1]s.relispartition AND %[1]s.relkind <> 'p'
        AND EXISTS (
            SELECT 1
            FROM pg_catalog.pg_inherits inh
            JOIN pg_catalog.pg_class parent ON parent.oid = inh.inhparent
            JOIN pg_catalog.pg_namespace pn ON pn.oid = parent.relnamespace
            WHERE inh.inhrelid = %[1]s.oid
              %[2]s
              %[3]s
              %[4]s
              %[5]s
        )
    )`,
		relAlias,
		f.onSchema("pn.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "parent.oid"),
		f.onAccessible(relationObject, "parent.oid"),
		f.onUserOwned("parent.relowner"),
	)
}

// queryArgs returns the positional query arguments referenced by the SQL
// fragments this filter emits. When a single schema is requested, the
// schema name is bound as `$1` (see onSchema) rather than interpolated, so
// arbitrary schema names are safe. Every buildXxxQuery uses onSchema at
// most once, so this is either empty or a single-element slice.
func (f schemaFilter) queryArgs() []any {
	if f.schema != "" {
		return []any{f.schema}
	}
	return nil
}

// systemSchemaExclusions returns the " AND <col> ..." clauses that drop the
// catalog schemas, TimescaleDB internals, information_schema, and the toolkit
// experimental schema from a default browse. col is the SQL expression naming
// the schema's name column (e.g. `n.nspname`, `nspname`). Shared by onSchema
// and checkSchemaExists so the default-browse exclusions stay in lockstep.
// Matches what popsql uses for the same purpose.
func systemSchemaExclusions(col string) string {
	var b strings.Builder
	fmt.Fprintf(&b, ` AND %s !~ '^pg_'`, col)
	fmt.Fprintf(&b, ` AND %s <> 'information_schema'`, col)
	fmt.Fprintf(&b, ` AND %s !~ '^_?timescaledb_'`, col)
	fmt.Fprintf(&b, ` AND %s <> 'toolkit_experimental'`, col)
	return b.String()
}

// onSchema returns " AND <col> = $1 AND <col> NOT LIKE 'pg_%' ..." type
// clauses. The caller is responsible for placing this in a WHERE context
// and passing queryArgs() to the query so `$1` is bound to the schema
// name.
func (f schemaFilter) onSchema(col string) string {
	// An explicit --schema request targets that namespace directly. The
	// standard exclusions must not apply, or requesting a system schema
	// (e.g. pg_catalog) would always return an empty result.
	if f.schema != "" {
		return fmt.Sprintf(" AND %s = $1", col)
	}
	if f.includeInternal {
		return ""
	}
	return systemSchemaExclusions(col)
}

// onExtensionObject returns a clause that excludes objects whose OID is
// referenced by a pg_depend entry with deptype = 'e' — i.e. objects that
// were created as part of an extension. classidExpr is the SQL expression
// identifying the catalog the object lives in (e.g. `'pg_class'::regclass`)
// and oidExpr is the SQL expression for the object's OID.
func (f schemaFilter) onExtensionObject(classidExpr, oidExpr string) string {
	if f.includeInternal {
		return ""
	}
	return fmt.Sprintf(`
        AND NOT EXISTS (
            SELECT 1 FROM pg_catalog.pg_depend dep
            WHERE dep.classid = %s
              AND dep.objid = %s
              AND dep.deptype = 'e'
        )`, classidExpr, oidExpr)
}

// objectKind identifies the privilege class onAccessible uses to test
// whether the current user can access an object. Each kind maps to the
// appropriate `has_*_privilege` catalog function.
type objectKind int

const (
	// relationObject covers tables, views, materialized views, and the
	// tables that triggers/partitions hang off of. Visibility is gated on
	// any table-level privilege.
	relationObject objectKind = iota
	// typeObject covers user-defined types such as enums. Visibility is
	// gated on the USAGE privilege.
	typeObject
	// routineObject covers functions and procedures. Visibility is gated on
	// the EXECUTE privilege.
	routineObject
)

// onAccessible returns a clause that keeps only objects the current user
// can access, using the privilege class appropriate to kind. oidCol is the
// SQL expression for the object's own OID (e.g. `c.oid`, `t.oid`,
// `p.oid`). This is what scopes the schema to "objects the user has access
// to": it keeps objects the user owns *or* has been GRANTed access to, and
// drops objects the user cannot touch (e.g. platform-managed helpers the
// user has no privilege on). When IncludeInternal is set, no clause is
// emitted so the full catalog is returned.
func (f schemaFilter) onAccessible(kind objectKind, oidCol string) string {
	if f.includeInternal {
		return ""
	}
	switch kind {
	case typeObject:
		return fmt.Sprintf(" AND pg_catalog.has_type_privilege(current_user, %s, 'USAGE')", oidCol)
	case routineObject:
		return fmt.Sprintf(" AND pg_catalog.has_function_privilege(current_user, %s, 'EXECUTE')", oidCol)
	default:
		return fmt.Sprintf(" AND pg_catalog.has_table_privilege(current_user, %s, 'SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER')", oidCol)
	}
}

// onUserOwned returns a clause that excludes objects owned by a superuser
// role. On Tiger Cloud the connecting user (e.g. tsdbadmin) is never a
// superuser, so superuser-owned objects are platform-managed helpers (e.g.
// the `postgres`-owned functions in `public`/`timescale_functions` that
// aren't extension-owned and so slip past onExtensionObject). ownerCol is
// the SQL expression identifying the owner role OID (e.g. `c.relowner`,
// `p.proowner`, `t.typowner`).
//
// This is only emitted on the default browse: like onSchema's name
// exclusions, it is dropped when an explicit --schema is requested (so
// `--schema pg_catalog`, whose objects are all superuser-owned, still
// returns results) or when IncludeInternal is set.
//
// Objects owned by the connecting user are never excluded, even if that user
// happens to be a superuser. On Tiger Cloud the connecting role is never a
// superuser so this is a no-op, but on self-hosted/dev databases the user may
// connect as a superuser; without this guard every object they created would
// be treated as a platform-managed helper and a default browse would return
// nothing. Helpers owned by *other* superusers (e.g. `postgres`) are still
// excluded.
func (f schemaFilter) onUserOwned(ownerCol string) string {
	if f.includeInternal || f.schema != "" {
		return ""
	}
	return fmt.Sprintf(`
        AND NOT EXISTS (
            SELECT 1 FROM pg_catalog.pg_roles r
            WHERE r.oid = %s
              AND r.rolsuper
              AND r.rolname <> current_user
        )`, ownerCol)
}

// Row types for scanning query results

type relationColumnRow struct {
	SchemaName   string  `db:"schema_name"`
	RelationName string  `db:"relation_name"`
	RelationType string  `db:"relation_type"`
	ColumnName   string  `db:"column_name"`
	DataType     string  `db:"data_type"`
	NotNull      bool    `db:"not_null"`
	DefaultValue *string `db:"default_value"`
	ColumnOrder  int16   `db:"column_order"`
	SequenceName *string `db:"sequence_name"`
	IdentityType string  `db:"identity_type"`
}

type viewDefinitionRow struct {
	SchemaName     string `db:"schema_name"`
	RelationName   string `db:"relation_name"`
	RelationKind   string `db:"relation_kind"`
	ViewDefinition string `db:"view_definition"`
}

type constraintRow struct {
	SchemaName     string   `db:"schema_name"`
	TableName      string   `db:"table_name"`
	ConstraintName string   `db:"constraint_name"`
	ConstraintType string   `db:"constraint_type"`
	Columns        []string `db:"columns"`
	RefTable       *string  `db:"ref_table"`
	RefColumns     []string `db:"ref_columns"`
	ConstraintDef  string   `db:"constraint_def"`
}

type indexRow struct {
	SchemaName  string  `db:"schema_name"`
	TableName   string  `db:"table_name"`
	IndexName   string  `db:"index_name"`
	IsUnique    bool    `db:"is_unique"`
	ColumnsDef  string  `db:"columns_def"`
	Definition  *string `db:"definition"`
	WhereClause *string `db:"where_clause"`
}

type enumRow struct {
	SchemaName string   `db:"schema_name"`
	EnumName   string   `db:"enum_name"`
	EnumValues []string `db:"enum_values"`
}

type triggerRow struct {
	SchemaName   string  `db:"schema_name"`
	TableName    string  `db:"table_name"`
	TriggerName  string  `db:"trigger_name"`
	Timing       *string `db:"timing"`
	Manipulation *string `db:"manipulation"`
	ActionStmt   *string `db:"action_statement"`
}

type routineRow struct {
	SchemaName  string  `db:"schema_name"`
	RoutineName string  `db:"routine_name"`
	RoutineArgs string  `db:"routine_args"`
	RoutineType string  `db:"routine_type"`
	Definition  *string `db:"routine_definition"`
}

type hypertableRow struct {
	SchemaName         string `db:"schema_name"`
	TableName          string `db:"table_name"`
	CompressionEnabled bool   `db:"compression_enabled"`
	NumChunks          int    `db:"num_chunks"`
}

type partitionRow struct {
	SchemaName      string `db:"schema_name"`
	TableName       string `db:"table_name"`
	PartitionName   string `db:"partition_name"`
	PartitionSchema string `db:"partition_schema"`
	PartitionBound  string `db:"partition_bound"`
}

// SQL queries are built dynamically because they need to splice in
// per-call filter clauses (schema-name restriction, internal filtering).
// Each builder returns a fully-formed query string ready for the driver.

func buildRelationsAndColumnsQuery(f schemaFilter) string {
	return fmt.Sprintf(`
SELECT
    n.nspname AS schema_name,
    c.relname AS relation_name,
    CASE c.relkind
        WHEN 'r' THEN 'table'
        WHEN 'p' THEN 'table'
        WHEN 'v' THEN 'view'
        WHEN 'm' THEN 'materialized_view'
    END AS relation_type,
    a.attname AS column_name,
    pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
    a.attnotnull AS not_null,
    pg_get_expr(d.adbin, d.adrelid) AS default_value,
    a.attnum AS column_order,
    pg_get_serial_sequence(format('%%I.%%I', n.nspname, c.relname), a.attname) AS sequence_name,
    a.attidentity::text AS identity_type
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
-- Include partitioned tables (relkind 'p') as tables. Leaf partitions are
-- normally hidden as standalone tables and surfaced under their parent's
-- Partitions list instead (see leafPartitionExclusion, which makes an
-- exception for cross-schema leaves when a single schema is requested).
-- Intermediate partitioned tables in a multi-level hierarchy (relispartition
-- children that are themselves partitioned, relkind 'p') ARE kept, so their
-- own sub-partitions remain reachable.
WHERE c.relkind IN ('r', 'p', 'v', 'm')
  %s
  AND a.attnum > 0
  AND NOT a.attisdropped
  %s
  %s
  %s
  %s
ORDER BY n.nspname, c.relname, a.attnum`,
		f.leafPartitionExclusion("c"),
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "c.oid"),
		f.onAccessible(relationObject, "c.oid"),
		f.onUserOwned("c.relowner"),
	)
}

// buildViewDefinitionsQuery fetches the defining SELECT for each view and
// materialized view, one row per relation. It is kept separate from the
// relations/columns query so pg_get_viewdef is evaluated once per view rather
// than once per column (a wide view would otherwise deparse its definition
// dozens of times, only for the duplicates to be discarded). It is only run
// when definitions are requested.
func buildViewDefinitionsQuery(f schemaFilter) string {
	return fmt.Sprintf(`
SELECT
    n.nspname AS schema_name,
    c.relname AS relation_name,
    c.relkind::text AS relation_kind,
    pg_get_viewdef(c.oid, true) AS view_definition
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('v', 'm')
  %s
  %s
  %s
  %s
ORDER BY n.nspname, c.relname`,
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "c.oid"),
		f.onAccessible(relationObject, "c.oid"),
		f.onUserOwned("c.relowner"),
	)
}

func buildConstraintsQuery(f schemaFilter) string {
	return fmt.Sprintf(`
SELECT
    n.nspname AS schema_name,
    c.relname AS table_name,
    con.conname AS constraint_name,
    con.contype::text AS constraint_type,
    (
        SELECT array_agg(a.attname ORDER BY x.n)
        FROM unnest(con.conkey) WITH ORDINALITY AS x(key, n)
        JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = x.key
    ) AS columns,
    -- Schema-qualify the referenced table only when it lives in a
    -- different schema than the constraint's table, so cross-schema
    -- foreign keys are unambiguous while same-schema ones stay terse.
    CASE
        WHEN confrel.oid IS NULL THEN NULL
        WHEN confreln.nspname = n.nspname THEN confrel.relname
        ELSE confreln.nspname || '.' || confrel.relname
    END AS ref_table,
    (
        SELECT array_agg(a.attname ORDER BY x.n)
        FROM unnest(con.confkey) WITH ORDINALITY AS x(key, n)
        JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = x.key
    ) AS ref_columns,
    pg_get_constraintdef(con.oid) AS constraint_def
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_class confrel ON confrel.oid = con.confrelid
LEFT JOIN pg_namespace confreln ON confreln.oid = confrel.relnamespace
WHERE con.contype IN ('p', 'u', 'f', 'c', 'x')
  %s
  %s
  %s
  %s
  %s
ORDER BY n.nspname, c.relname, con.contype, con.conname`,
		// Skip constraints on leaf partitions whose parent is in scope: those
		// constraints are clones of the parent's and would be discarded anyway
		// (fetchConstraints drops rows whose table isn't in tableIndex). The
		// same predicate keeps a cross-schema standalone leaf's constraints.
		f.leafPartitionExclusion("c"),
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "c.oid"),
		f.onAccessible(relationObject, "c.oid"),
		f.onUserOwned("c.relowner"),
	)
}

func buildIndexesQuery(f schemaFilter) string {
	return fmt.Sprintf(`
SELECT
    n.nspname AS schema_name,
    t.relname AS table_name,
    i.relname AS index_name,
    ix.indisunique AS is_unique,
    (
        SELECT string_agg(
            pg_get_indexdef(ix.indexrelid, k.n, false) ||
            CASE (ix.indoption[k.n - 1] & 3)
                WHEN 0 THEN ''
                WHEN 1 THEN ' DESC NULLS LAST'
                WHEN 2 THEN ' NULLS FIRST'
                WHEN 3 THEN ' DESC'
            END,
            ', ' ORDER BY k.n
        )
        FROM generate_series(1, ix.indnkeyatts) AS k(n)
    ) AS columns_def,
    %s AS definition,
    pg_get_expr(ix.indpred, ix.indrelid) AS where_clause
FROM pg_index ix
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE t.relkind IN ('r', 'p', 'm')
  -- Mirror the relations query: hide leaf partitions but keep intermediate
  -- partitioned tables so their indexes stay visible. Use the same
  -- schema-aware exclusion (leafPartitionExclusion) so a cross-schema leaf
  -- that the relations query surfaces standalone also has its indexes
  -- listed here, rather than being silently dropped.
  %s
  -- Exclude indexes that back a constraint (PRIMARY KEY / UNIQUE /
  -- EXCLUDE). Those are surfaced via the constraints query, so listing
  -- them here as well would duplicate them.
  AND NOT EXISTS (
      SELECT 1 FROM pg_constraint con
      WHERE con.conindid = ix.indexrelid
  )
  %s
  %s
  %s
  %s
ORDER BY n.nspname, t.relname, i.relname`,
		// Gate the full CREATE INDEX text behind --definitions, like views and
		// routines: it can embed expression/partial-index SQL the caller hasn't
		// asked for. The columns_def list above is always emitted because it's
		// the core display info for the index.
		f.definitionExpr("pg_get_indexdef(ix.indexrelid)"),
		f.leafPartitionExclusion("t"),
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "t.oid"),
		f.onAccessible(relationObject, "t.oid"),
		f.onUserOwned("t.relowner"),
	)
}

func buildEnumsQuery(f schemaFilter) string {
	return fmt.Sprintf(`
SELECT
    n.nspname AS schema_name,
    t.typname AS enum_name,
    array_agg(e.enumlabel ORDER BY e.enumsortorder) AS enum_values
FROM pg_type t
JOIN pg_namespace n ON n.oid = t.typnamespace
JOIN pg_enum e ON e.enumtypid = t.oid
WHERE TRUE
  %s
  %s
  %s
  %s
GROUP BY n.nspname, t.typname
ORDER BY n.nspname, t.typname`,
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_type'::regclass", "t.oid"),
		f.onAccessible(typeObject, "t.oid"),
		f.onUserOwned("t.typowner"),
	)
}

func buildTriggersQuery(f schemaFilter) string {
	// We read triggers straight from pg_catalog.pg_trigger rather than
	// information_schema.triggers. information_schema omits statement-level
	// TRUNCATE triggers entirely and only surfaces triggers on tables the
	// current user has a privilege *other than* SELECT on — so a trigger on a
	// SELECT-only table the rest of the schema output happily shows would be
	// silently dropped. Reading the catalog directly avoids both gaps and lets
	// us apply the same OID-based filters used for every other object kind:
	// excluding extension-owned triggers (onExtensionObject), triggers on
	// tables the user can't access (onAccessible), and internally generated
	// triggers (tgisinternal).
	//
	// pg_trigger.tgtype is a bitmask encoding both the timing and the set of
	// firing events for a single trigger, so a trigger that fires on multiple
	// events (e.g. INSERT OR UPDATE) is one catalog row. To preserve the
	// one-row-per-manipulation shape the tree/format code expects (mirroring
	// information_schema's layout), we expand each trigger across the possible
	// events via a lateral VALUES join, keeping only the bits that are set.
	// The action statement (e.g. "EXECUTE FUNCTION foo()") is sliced out of
	// pg_get_triggerdef, which is the only catalog source for the formatted
	// call including its arguments.
	return fmt.Sprintf(`
SELECT
    n.nspname AS schema_name,
    c.relname AS table_name,
    tg.tgname AS trigger_name,
    CASE
        WHEN (tg.tgtype::int & 64) <> 0 THEN 'INSTEAD OF'
        WHEN (tg.tgtype::int & 2) <> 0 THEN 'BEFORE'
        ELSE 'AFTER'
    END AS timing,
    ev.manipulation AS manipulation,
    substring(
        pg_catalog.pg_get_triggerdef(tg.oid)
        FROM 'EXECUTE (?:FUNCTION|PROCEDURE) .*$'
    ) AS action_statement
FROM pg_catalog.pg_trigger tg
JOIN pg_catalog.pg_class c ON c.oid = tg.tgrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
CROSS JOIN LATERAL (
    VALUES (4, 'INSERT'), (8, 'DELETE'), (16, 'UPDATE'), (32, 'TRUNCATE')
) AS ev(bit, manipulation)
WHERE NOT tg.tgisinternal
  AND (tg.tgtype::int & ev.bit) <> 0
  %s
  %s
  %s
  %s
  %s
ORDER BY schema_name, table_name, trigger_name, manipulation`,
		// Skip triggers on leaf partitions whose parent is in scope: those are
		// clones of the parent's triggers and would be discarded anyway
		// (fetchTriggers drops rows whose table isn't in tableIndex). The same
		// predicate keeps a cross-schema standalone leaf's triggers.
		f.leafPartitionExclusion("c"),
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_trigger'::regclass", "tg.oid"),
		f.onAccessible(relationObject, "c.oid"),
		f.onUserOwned("c.relowner"),
	)
}

func buildRoutinesQuery(f schemaFilter) string {
	// pg_proc.prokind: 'f' = function, 'p' = procedure, 'a' = aggregate,
	// 'w' = window. We surface plain functions and procedures only.
	return fmt.Sprintf(`
SELECT
    n.nspname AS schema_name,
    p.proname AS routine_name,
    pg_get_function_identity_arguments(p.oid) AS routine_args,
    CASE p.prokind
        WHEN 'f' THEN 'FUNCTION'
        WHEN 'p' THEN 'PROCEDURE'
    END AS routine_type,
    %s AS routine_definition
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind IN ('f', 'p')
  %s
  %s
  %s
  %s
ORDER BY n.nspname, p.proname, routine_args`,
		f.definitionExpr("pg_get_functiondef(p.oid)"),
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_proc'::regclass", "p.oid"),
		f.onAccessible(routineObject, "p.oid"),
		f.onUserOwned("p.proowner"),
	)
}

// hypertablesQuery returns hypertable metadata for the requested schemas.
// Caller must verify the timescaledb extension is installed before
// running this; otherwise the query errors with "relation does not exist".
func buildHypertablesQuery(f schemaFilter) string {
	return fmt.Sprintf(`
SELECT
    h.hypertable_schema AS schema_name,
    h.hypertable_name AS table_name,
    h.compression_enabled,
    COALESCE(h.num_chunks, 0) AS num_chunks
FROM timescaledb_information.hypertables h
WHERE TRUE
  %s
ORDER BY h.hypertable_schema, h.hypertable_name`,
		f.onSchema("h.hypertable_schema"),
	)
}

// buildPartitionsQuery returns the direct child partitions of each
// partitioned table (relkind 'p'), one row per child, along with the
// child's bound expression. In a multi-level hierarchy each level yields
// its own rows (e.g. top->intermediate and intermediate->leaf); because
// intermediate partitioned tables are kept in the relations query, every
// parent resolves in tableIndex and no level is dropped. The same
// OID-based exclusion filters used elsewhere are applied to the parent
// table so extension-owned and inaccessible partition hierarchies are
// filtered consistently.
func buildPartitionsQuery(f schemaFilter) string {
	return fmt.Sprintf(`
SELECT
    pn.nspname AS schema_name,
    parent.relname AS table_name,
    child.relname AS partition_name,
    cn.nspname AS partition_schema,
    COALESCE(pg_get_expr(child.relpartbound, child.oid), '') AS partition_bound
FROM pg_inherits inh
JOIN pg_class parent ON parent.oid = inh.inhparent
JOIN pg_namespace pn ON pn.oid = parent.relnamespace
JOIN pg_class child ON child.oid = inh.inhrelid
JOIN pg_namespace cn ON cn.oid = child.relnamespace
WHERE parent.relkind = 'p'
  %s
  %s
  %s
  %s
ORDER BY pn.nspname, parent.relname, child.relname`,
		f.onSchema("pn.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "parent.oid"),
		f.onAccessible(relationObject, "parent.oid"),
		f.onUserOwned("parent.relowner"),
	)
}

// FetchDatabaseSchema fetches the complete schema information for a
// database. By default only user-visible schemas and objects are returned;
// pass IncludeInternal=true to include catalog, TimescaleDB internals, and
// extension-owned objects. Pass Schema to limit results to a single
// namespace. View/routine definitions are omitted unless
// IncludeDefinitions=true.
func FetchDatabaseSchema(ctx context.Context, args FetchDatabaseSchemaArgs) (*DatabaseSchema, error) {
	database, err := fetchDatabase(ctx, args.Client, args.ProjectID, args.DatabaseRef)
	if err != nil {
		return nil, err
	}

	if err := CheckReady(database); err != nil {
		return nil, err
	}

	conn, err := connectToDatabase(ctx, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.Background())

	if args.Schema != "" {
		if err := checkSchemaExists(ctx, conn, args.Schema, args.IncludeInternal); err != nil {
			return nil, err
		}
	}

	filter := schemaFilter{
		includeInternal:    args.IncludeInternal,
		includeDefinitions: args.IncludeDefinitions,
		schema:             args.Schema,
	}

	// Build the schema in stages: first collect every object keyed by
	// (schema, name) in flat maps, then attach constraints/indexes/triggers,
	// then assemble the final NamespacedSchema slice in name order.
	bld := newSchemaBuilder()

	if err := fetchRelationsAndColumns(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch relations: %w", err)
	}
	if err := fetchViewDefinitions(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch view definitions: %w", err)
	}
	if err := fetchConstraints(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch constraints: %w", err)
	}
	if err := fetchIndexes(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch indexes: %w", err)
	}
	if err := fetchTriggers(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch triggers: %w", err)
	}
	if err := fetchEnums(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch enums: %w", err)
	}
	if err := fetchRoutines(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch routines: %w", err)
	}
	if err := fetchHypertables(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch hypertables: %w", err)
	}
	if err := fetchPartitions(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch partitions: %w", err)
	}

	return &DatabaseSchema{
		ID:      database.Id,
		Name:    database.Name,
		Schemas: bld.build(),
	}, nil
}

// SchemaNotFoundError indicates the requested namespace does not exist. It
// carries a friendly message listing the available schemas when they could be
// enumerated. Callers can detect it with errors.As to distinguish a mistyped
// schema (a client input error) from an upstream/connection failure.
type SchemaNotFoundError struct {
	// Schema is the requested namespace that was not found.
	Schema string
	// Available lists the schemas the connecting user can access (i.e. holds
	// USAGE on), minus the internal namespaces a default browse hides unless
	// --internal is set. It is a best-effort suggestion list, not a guarantee
	// that each schema would produce non-empty results (a schema whose
	// contents are entirely extension-owned still renders empty on a default
	// browse). It is nil when enumeration failed (in which case ListErr is
	// set).
	Available []string
	// ListErr is non-nil when listing the available schemas failed.
	ListErr error
}

// Error implements the error interface.
func (e *SchemaNotFoundError) Error() string {
	switch {
	case e.ListErr != nil:
		return fmt.Sprintf("schema %q not found (failed to list available schemas: %v)", e.Schema, e.ListErr)
	case len(e.Available) == 0:
		return fmt.Sprintf("schema %q not found", e.Schema)
	default:
		return fmt.Sprintf("schema %q not found; available schemas: %s", e.Schema, strings.Join(e.Available, ", "))
	}
}

// Unwrap exposes the underlying listing error (if any) for errors.Is/As.
func (e *SchemaNotFoundError) Unwrap() error { return e.ListErr }

// checkSchemaExists verifies the requested namespace exists, returning a
// *SchemaNotFoundError listing the available schemas if it does not. This
// keeps an empty result for a mistyped --schema from looking like an empty
// database.
//
// includeInternal mirrors the caller's --internal flag: an explicit --schema
// request can legitimately target a system/internal namespace (onSchema drops
// the standard exclusions for explicit schemas), so when --internal is set the
// suggestion list includes those schemas too. When it is unset we suggest only
// the user-facing schemas that a default browse would surface, to avoid
// drowning the hint in catalog/TimescaleDB-internal namespaces.
func checkSchemaExists(ctx context.Context, conn *pgx.Conn, schema string, includeInternal bool) error {
	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		schema,
	).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check schema existence: %w", err)
	}
	if exists {
		return nil
	}

	// Only suggest schemas the connecting user can actually access: a schema
	// they have no USAGE privilege on would yield an empty browse, so
	// pointing them at it is unhelpful. The name exclusions additionally
	// mirror schemaFilter.onSchema's default-browse filtering; with --internal
	// we drop them so internal namespaces are suggested too (but still gate on
	// USAGE).
	query := `SELECT nspname FROM pg_namespace
	 WHERE pg_catalog.has_schema_privilege(current_user, oid, 'USAGE')
	 ORDER BY nspname`
	if !includeInternal {
		query = `SELECT nspname FROM pg_namespace
		 WHERE pg_catalog.has_schema_privilege(current_user, oid, 'USAGE')` +
			systemSchemaExclusions("nspname") + `
		 ORDER BY nspname`
	}
	rows, err := conn.Query(ctx, query)
	if err != nil {
		return &SchemaNotFoundError{Schema: schema, ListErr: err}
	}
	available, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return &SchemaNotFoundError{Schema: schema, ListErr: err}
	}
	return &SchemaNotFoundError{Schema: schema, Available: available}
}

// connectToDatabase establishes a connection to the given database.
func connectToDatabase(ctx context.Context, database api.Database) (*pgx.Conn, error) {
	const role = "tsdbadmin"

	password, err := GetPassword(database, role)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve password: %w", err)
	}

	connStr, err := BuildConnectionString(ConnectionStringArgs{
		Database: database,
		Role:     role,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return conn, nil
}

// schemaBuilder collects per-schema objects as we run queries.
type schemaBuilder struct {
	// schemaName -> namespace contents
	namespaces map[string]*NamespacedSchema
	// (schema, name) -> table pointer (so subsequent queries can attach
	// constraints/indexes/triggers/hypertable info to the right object)
	tableIndex   map[qualifiedName]*TableSchema
	viewIndex    map[qualifiedName]*ViewSchema
	matViewIndex map[qualifiedName]*ViewSchema
}

type qualifiedName struct {
	Schema string
	Name   string
}

func newSchemaBuilder() *schemaBuilder {
	return &schemaBuilder{
		namespaces:   make(map[string]*NamespacedSchema),
		tableIndex:   make(map[qualifiedName]*TableSchema),
		viewIndex:    make(map[qualifiedName]*ViewSchema),
		matViewIndex: make(map[qualifiedName]*ViewSchema),
	}
}

func (b *schemaBuilder) namespace(name string) *NamespacedSchema {
	ns, ok := b.namespaces[name]
	if !ok {
		ns = &NamespacedSchema{Name: name}
		b.namespaces[name] = ns
	}
	return ns
}

func (b *schemaBuilder) build() []NamespacedSchema {
	if len(b.namespaces) == 0 {
		return nil
	}
	out := make([]NamespacedSchema, 0, len(b.namespaces))
	for _, ns := range b.namespaces {
		out = append(out, *ns)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func fetchRelationsAndColumns(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	rows, err := conn.Query(ctx, buildRelationsAndColumnsQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[relationColumnRow])
	if err != nil {
		return err
	}

	// Collect into per-namespace maps keyed by relation name; we'll flatten
	// to sorted slices below. Buffering avoids the slice-pointer
	// invalidation hazard from appending to a NamespacedSchema field while
	// also keeping a pointer into it.
	type relBuf struct {
		tables   map[string]*TableSchema
		views    map[string]*ViewSchema
		matViews map[string]*ViewSchema
	}
	perNS := make(map[string]*relBuf)
	getBuf := func(schema string) *relBuf {
		buf, ok := perNS[schema]
		if !ok {
			buf = &relBuf{
				tables:   make(map[string]*TableSchema),
				views:    make(map[string]*ViewSchema),
				matViews: make(map[string]*ViewSchema),
			}
			perNS[schema] = buf
		}
		return buf
	}

	for _, row := range results {
		buf := getBuf(row.SchemaName)
		switch row.RelationType {
		case "table":
			t, ok := buf.tables[row.RelationName]
			if !ok {
				t = &TableSchema{Name: row.RelationName}
				buf.tables[row.RelationName] = t
			}
			t.Columns = append(t.Columns, TableColumnSchema{
				Name:         row.ColumnName,
				Type:         row.DataType,
				NotNull:      row.NotNull,
				Default:      util.DerefStr(row.DefaultValue),
				IsSerial:     row.SequenceName != nil && row.IdentityType == "",
				IdentityType: row.IdentityType,
			})
		case "view":
			v, ok := buf.views[row.RelationName]
			if !ok {
				v = &ViewSchema{Name: row.RelationName}
				buf.views[row.RelationName] = v
			}
			v.Columns = append(v.Columns, ViewColumnSchema{Name: row.ColumnName, Type: row.DataType})
		case "materialized_view":
			mv, ok := buf.matViews[row.RelationName]
			if !ok {
				mv = &ViewSchema{Name: row.RelationName}
				buf.matViews[row.RelationName] = mv
			}
			mv.Columns = append(mv.Columns, ViewColumnSchema{Name: row.ColumnName, Type: row.DataType})
		}
	}

	// Flatten into sorted slices on each NamespacedSchema. The pointer-index
	// step at the bottom captures stable addresses into the final slices.
	for schemaName, buf := range perNS {
		ns := b.namespace(schemaName)
		tableNames := sortedKeys(buf.tables)
		for _, name := range tableNames {
			ns.Tables = append(ns.Tables, *buf.tables[name])
		}
		viewNames := sortedKeys(buf.views)
		for _, name := range viewNames {
			ns.Views = append(ns.Views, *buf.views[name])
		}
		mvNames := sortedKeys(buf.matViews)
		for _, name := range mvNames {
			ns.MaterializedViews = append(ns.MaterializedViews, *buf.matViews[name])
		}
	}

	// Capture pointers into the final slices so later fetch steps (indexes,
	// constraints, triggers, hypertables) can attach to the right object.
	for _, ns := range b.namespaces {
		for i := range ns.Tables {
			b.tableIndex[qualifiedName{Schema: ns.Name, Name: ns.Tables[i].Name}] = &ns.Tables[i]
		}
		for i := range ns.Views {
			b.viewIndex[qualifiedName{Schema: ns.Name, Name: ns.Views[i].Name}] = &ns.Views[i]
		}
		for i := range ns.MaterializedViews {
			b.matViewIndex[qualifiedName{Schema: ns.Name, Name: ns.MaterializedViews[i].Name}] = &ns.MaterializedViews[i]
		}
	}

	return nil
}

// fetchViewDefinitions attaches the defining SELECT to each view and
// materialized view. It must run after fetchRelationsAndColumns, which
// populates the viewIndex/matViewIndex pointer maps it attaches to. It is a
// no-op unless definitions were requested, so the pg_get_viewdef work is
// skipped entirely on the default browse.
func fetchViewDefinitions(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	if !f.includeDefinitions {
		return nil
	}
	rows, err := conn.Query(ctx, buildViewDefinitionsQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[viewDefinitionRow])
	if err != nil {
		return err
	}

	for _, row := range results {
		qn := qualifiedName{Schema: row.SchemaName, Name: row.RelationName}
		def := strings.TrimSpace(row.ViewDefinition)
		// relkind 'v' is a plain view, 'm' is a materialized view.
		if row.RelationKind == "m" {
			if mv, ok := b.matViewIndex[qn]; ok {
				mv.Definition = def
			}
		} else if v, ok := b.viewIndex[qn]; ok {
			v.Definition = def
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fetchConstraints(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	rows, err := conn.Query(ctx, buildConstraintsQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[constraintRow])
	if err != nil {
		return err
	}

	for _, row := range results {
		t, ok := b.tableIndex[qualifiedName{Schema: row.SchemaName, Name: row.TableName}]
		if !ok {
			continue
		}
		switch row.ConstraintType {
		case "p":
			t.Constraints = append(t.Constraints, TableConstraint{
				Type:    ConstraintPrimaryKey,
				Name:    row.ConstraintName,
				Columns: row.Columns,
			})
		case "u":
			t.Constraints = append(t.Constraints, TableConstraint{
				Type:    ConstraintUnique,
				Name:    row.ConstraintName,
				Columns: row.Columns,
			})
		case "f":
			t.Constraints = append(t.Constraints, TableConstraint{
				Type:       ConstraintForeignKey,
				Name:       row.ConstraintName,
				Columns:    row.Columns,
				RefTable:   util.DerefStr(row.RefTable),
				RefColumns: row.RefColumns,
			})
		case "c":
			t.Checks = append(t.Checks, CheckConstraint{
				Name:       row.ConstraintName,
				Columns:    row.Columns,
				Expression: row.ConstraintDef,
			})
		case "x":
			t.Exclusions = append(t.Exclusions, ExclusionConstraint{
				Name:       row.ConstraintName,
				Definition: row.ConstraintDef,
			})
		}
	}
	return nil
}

func fetchIndexes(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	rows, err := conn.Query(ctx, buildIndexesQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[indexRow])
	if err != nil {
		return err
	}

	for _, row := range results {
		idx := IndexSchema{
			Name:        row.IndexName,
			Columns:     row.ColumnsDef,
			Definition:  util.DerefStr(row.Definition),
			IsUnique:    row.IsUnique,
			WhereClause: util.DerefStr(row.WhereClause),
		}
		qn := qualifiedName{Schema: row.SchemaName, Name: row.TableName}
		if t, ok := b.tableIndex[qn]; ok {
			t.Indexes = append(t.Indexes, idx)
		} else if mv, ok := b.matViewIndex[qn]; ok {
			mv.Indexes = append(mv.Indexes, idx)
		}
	}
	return nil
}

func fetchTriggers(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	rows, err := conn.Query(ctx, buildTriggersQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[triggerRow])
	if err != nil {
		return err
	}

	for _, row := range results {
		qn := qualifiedName{Schema: row.SchemaName, Name: row.TableName}
		trigger := TriggerSchema{
			Name:         row.TriggerName,
			Timing:       util.DerefStr(row.Timing),
			Manipulation: util.DerefStr(row.Manipulation),
			Statement:    util.DerefStr(row.ActionStmt),
		}
		// Triggers can live on tables or on views (e.g. INSTEAD OF
		// triggers). Attach to whichever the event object is.
		if t, ok := b.tableIndex[qn]; ok {
			t.Triggers = append(t.Triggers, trigger)
		} else if v, ok := b.viewIndex[qn]; ok {
			v.Triggers = append(v.Triggers, trigger)
		}
	}
	return nil
}

func fetchPartitions(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	rows, err := conn.Query(ctx, buildPartitionsQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[partitionRow])
	if err != nil {
		return err
	}

	for _, row := range results {
		t, ok := b.tableIndex[qualifiedName{Schema: row.SchemaName, Name: row.TableName}]
		if !ok {
			continue
		}
		// Only record the partition's schema when it differs from its parent
		// table's schema; partitions normally share the parent's schema, but
		// PostgreSQL allows them to live elsewhere.
		partitionSchema := ""
		if row.PartitionSchema != row.SchemaName {
			partitionSchema = row.PartitionSchema
		}
		t.Partitions = append(t.Partitions, PartitionInfo{
			Name:   row.PartitionName,
			Schema: partitionSchema,
			Bound:  row.PartitionBound,
		})
	}
	return nil
}

func fetchEnums(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	rows, err := conn.Query(ctx, buildEnumsQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[enumRow])
	if err != nil {
		return err
	}

	// The query orders by (nspname, typname) and rows are appended in order,
	// so each namespace's Enums slice is already name-sorted (like
	// fetchRoutines, no Go-side sort is needed).
	for _, row := range results {
		ns := b.namespace(row.SchemaName)
		ns.Enums = append(ns.Enums, EnumSchema{
			Name:   row.EnumName,
			Values: row.EnumValues,
		})
	}
	return nil
}

func fetchRoutines(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	rows, err := conn.Query(ctx, buildRoutinesQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[routineRow])
	if err != nil {
		return err
	}

	for _, row := range results {
		ns := b.namespace(row.SchemaName)
		r := Routine{
			Name:       row.RoutineName,
			Arguments:  row.RoutineArgs,
			Type:       RoutineType(row.RoutineType),
			Definition: strings.TrimSpace(util.DerefStr(row.Definition)),
		}
		switch r.Type {
		case RoutineFunction:
			ns.Functions = append(ns.Functions, r)
		case RoutineProcedure:
			ns.Procedures = append(ns.Procedures, r)
		}
	}
	return nil
}

func fetchHypertables(ctx context.Context, conn *pgx.Conn, f schemaFilter, b *schemaBuilder) error {
	// Skip the query entirely if the timescaledb extension isn't installed.
	var hasExt bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')`,
	).Scan(&hasExt); err != nil {
		return err
	}
	if !hasExt {
		return nil
	}

	rows, err := conn.Query(ctx, buildHypertablesQuery(f), f.queryArgs()...)
	if err != nil {
		return err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[hypertableRow])
	if err != nil {
		return err
	}

	for _, row := range results {
		t, ok := b.tableIndex[qualifiedName{Schema: row.SchemaName, Name: row.TableName}]
		if !ok {
			continue
		}
		t.Hypertable = &HypertableInfo{
			CompressionEnabled: row.CompressionEnabled,
			NumChunks:          row.NumChunks,
		}
	}
	return nil
}
