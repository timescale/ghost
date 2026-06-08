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
	// Partitions lists the child partitions of a partitioned table. Only
	// populated for partitioned tables (relkind 'p'); the children
	// themselves are hidden as standalone tables.
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
}

// schemaFilter holds the SQL fragments needed to scope a query to the
// user-visible schemas / objects.
type schemaFilter struct {
	includeInternal bool
	schema          string
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
	var b strings.Builder
	// Standard exclusions: catalog schemas, TimescaleDB internals,
	// information_schema. Matches what popsql uses for the same purpose.
	fmt.Fprintf(&b, ` AND %s !~ '^pg_'`, col)
	fmt.Fprintf(&b, ` AND %s <> 'information_schema'`, col)
	fmt.Fprintf(&b, ` AND %s !~ '^_?timescaledb_'`, col)
	fmt.Fprintf(&b, ` AND %s <> 'toolkit_experimental'`, col)
	return b.String()
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

// onOwner returns a clause that excludes objects whose owner role the
// current session role isn't a member of. Filters out platform-managed
// objects (e.g. `postgres`-owned helper functions that ship with Tiger
// Cloud) while keeping anything the user can rightfully claim. ownerCol
// is the SQL expression identifying the owner role OID (e.g.
// `c.relowner`, `p.proowner`, `t.typowner`).
func (f schemaFilter) onOwner(ownerCol string) string {
	if f.includeInternal {
		return ""
	}
	return fmt.Sprintf(" AND pg_catalog.pg_has_role(current_user, %s, 'MEMBER')", ownerCol)
}

// Row types for scanning query results

type relationColumnRow struct {
	SchemaName     string  `db:"schema_name"`
	RelationName   string  `db:"relation_name"`
	RelationType   string  `db:"relation_type"`
	ColumnName     string  `db:"column_name"`
	DataType       string  `db:"data_type"`
	NotNull        bool    `db:"not_null"`
	DefaultValue   *string `db:"default_value"`
	ColumnOrder    int16   `db:"column_order"`
	SequenceName   *string `db:"sequence_name"`
	IdentityType   string  `db:"identity_type"`
	ViewDefinition *string `db:"view_definition"`
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
	Definition  string  `db:"definition"`
	WhereClause *string `db:"where_clause"`
}

type enumRow struct {
	SchemaName string   `db:"schema_name"`
	EnumName   string   `db:"enum_name"`
	EnumValues []string `db:"enum_values"`
}

type triggerRow struct {
	SchemaName   string `db:"schema_name"`
	TableName    string `db:"table_name"`
	TriggerName  string `db:"trigger_name"`
	Timing       string `db:"timing"`
	Manipulation string `db:"manipulation"`
	ActionStmt   string `db:"action_statement"`
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
    a.attidentity::text AS identity_type,
    CASE WHEN c.relkind IN ('v', 'm') THEN pg_get_viewdef(c.oid, true) END AS view_definition
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
-- Include partitioned tables (relkind 'p') as tables, but exclude child
-- partitions (relispartition): they're surfaced under the parent's
-- Partitions list rather than as standalone tables.
WHERE c.relkind IN ('r', 'p', 'v', 'm')
  AND NOT c.relispartition
  AND a.attnum > 0
  AND NOT a.attisdropped
  %s
  %s
  %s
ORDER BY n.nspname, c.relname, a.attnum`,
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "c.oid"),
		f.onOwner("c.relowner"),
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
ORDER BY n.nspname, c.relname, con.contype, con.conname`,
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "c.oid"),
		f.onOwner("c.relowner"),
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
    pg_get_indexdef(ix.indexrelid) AS definition,
    pg_get_expr(ix.indpred, ix.indrelid) AS where_clause
FROM pg_index ix
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE t.relkind IN ('r', 'p', 'm')
  AND NOT t.relispartition
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
ORDER BY n.nspname, t.relname, i.relname`,
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "t.oid"),
		f.onOwner("t.relowner"),
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
GROUP BY n.nspname, t.typname
ORDER BY n.nspname, t.typname`,
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_type'::regclass", "t.oid"),
		f.onOwner("t.typowner"),
	)
}

func buildTriggersQuery(f schemaFilter) string {
	// information_schema.triggers gives us the user-friendly, SQL-standard
	// columns (timing, one row per event_manipulation, and a formatted
	// action_statement), which the popsql tree also flattens into separate
	// entries. We join it to pg_trigger/pg_class/pg_namespace so we can apply
	// the same OID-based filters used for every other object kind: excluding
	// extension-owned triggers (onExtensionObject), triggers on tables owned
	// by platform roles (onOwner), and internally generated triggers
	// (tgisinternal). The join is on the trigger's identity (schema, table,
	// name), so it preserves information_schema's per-manipulation rows.
	return fmt.Sprintf(`
SELECT
    ist.trigger_schema AS schema_name,
    ist.event_object_table AS table_name,
    ist.trigger_name AS trigger_name,
    ist.action_timing AS timing,
    ist.event_manipulation AS manipulation,
    ist.action_statement AS action_statement
FROM information_schema.triggers ist
JOIN pg_catalog.pg_namespace n ON n.nspname = ist.event_object_schema
JOIN pg_catalog.pg_class c ON c.relname = ist.event_object_table AND c.relnamespace = n.oid
JOIN pg_catalog.pg_trigger tg ON tg.tgrelid = c.oid AND tg.tgname = ist.trigger_name
WHERE NOT tg.tgisinternal
  %s
  %s
  %s
ORDER BY schema_name, table_name, trigger_name, manipulation`,
		f.onSchema("ist.trigger_schema"),
		f.onExtensionObject("'pg_trigger'::regclass", "tg.oid"),
		f.onOwner("c.relowner"),
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
    pg_get_functiondef(p.oid) AS routine_definition
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind IN ('f', 'p')
  %s
  %s
  %s
ORDER BY n.nspname, p.proname, routine_args`,
		f.onSchema("n.nspname"),
		f.onExtensionObject("'pg_proc'::regclass", "p.oid"),
		f.onOwner("p.proowner"),
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

// buildPartitionsQuery returns the child partitions of each partitioned
// table (relkind 'p'), one row per child, along with the child's bound
// expression. The same OID-based exclusion filters used elsewhere are
// applied to the parent table so platform/extension-owned partition
// hierarchies are filtered consistently.
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
ORDER BY pn.nspname, parent.relname, child.relname`,
		f.onSchema("pn.nspname"),
		f.onExtensionObject("'pg_class'::regclass", "parent.oid"),
		f.onOwner("parent.relowner"),
	)
}

// FetchDatabaseSchema fetches the complete schema information for a
// database. By default only user-visible schemas and objects are returned;
// pass IncludeInternal=true to include catalog, TimescaleDB internals, and
// extension-owned objects. Pass Schema to limit results to a single
// namespace.
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
		if err := checkSchemaExists(ctx, conn, args.Schema); err != nil {
			return nil, err
		}
	}

	filter := schemaFilter{
		includeInternal: args.IncludeInternal,
		schema:          args.Schema,
	}

	// Build the schema in stages: first collect every object keyed by
	// (schema, name) in flat maps, then attach constraints/indexes/triggers,
	// then assemble the final NamespacedSchema slice in name order.
	bld := newSchemaBuilder()

	if err := fetchRelationsAndColumns(ctx, conn, filter, bld); err != nil {
		return nil, fmt.Errorf("failed to fetch relations: %w", err)
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

// checkSchemaExists verifies the requested namespace exists, returning a
// friendly error listing the available schemas if it does not. This keeps an
// empty result for a mistyped --schema from looking like an empty database.
func checkSchemaExists(ctx context.Context, conn *pgx.Conn, schema string) error {
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

	// Keep these exclusions in sync with schemaFilter.onSchema so the
	// suggested schemas are exactly those that produce non-empty results.
	rows, err := conn.Query(ctx,
		`SELECT nspname FROM pg_namespace
		 WHERE nspname !~ '^pg_'
		   AND nspname <> 'information_schema'
		   AND nspname !~ '^_?timescaledb_'
		   AND nspname <> 'toolkit_experimental'
		 ORDER BY nspname`,
	)
	if err != nil {
		return fmt.Errorf("schema %q not found (failed to list available schemas: %w)", schema, err)
	}
	available, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("schema %q not found (failed to list available schemas: %w)", schema, err)
	}
	if len(available) == 0 {
		return fmt.Errorf("schema %q not found", schema)
	}
	return fmt.Errorf("schema %q not found; available schemas: %s", schema, strings.Join(available, ", "))
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
				v = &ViewSchema{Name: row.RelationName, Definition: strings.TrimSpace(util.DerefStr(row.ViewDefinition))}
				buf.views[row.RelationName] = v
			}
			v.Columns = append(v.Columns, ViewColumnSchema{Name: row.ColumnName, Type: row.DataType})
		case "materialized_view":
			mv, ok := buf.matViews[row.RelationName]
			if !ok {
				mv = &ViewSchema{Name: row.RelationName, Definition: strings.TrimSpace(util.DerefStr(row.ViewDefinition))}
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
			Definition:  row.Definition,
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
			Timing:       row.Timing,
			Manipulation: row.Manipulation,
			Statement:    row.ActionStmt,
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

	for _, row := range results {
		ns := b.namespace(row.SchemaName)
		ns.Enums = append(ns.Enums, EnumSchema{
			Name:   row.EnumName,
			Values: row.EnumValues,
		})
	}
	for _, ns := range b.namespaces {
		sort.Slice(ns.Enums, func(i, j int) bool { return ns.Enums[i].Name < ns.Enums[j].Name })
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
