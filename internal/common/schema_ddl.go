package common

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// FormatSchemaDDL renders fetched schema objects as PostgreSQL DDL, suitable
// as the schema file for sqlc's hybrid analysis mode (schema file plus live
// database connection), avoiding a dependency on pg_dump.
//
// This DDL is an internal artifact fed to sqlc's catalog — it is never shown
// to users (ghost_schema output is separate and faithful). It only needs to
// be good enough for sqlc's catalog to resolve parameter names, types, and
// nullability; anything it omits or simplifies is backstopped by sqlc's live
// database analyzer. Full pg_dump fidelity is deliberately not the bar:
//
//   - Enum types are emitted first (CREATE TYPE ... AS ENUM), since the enum
//     values in the generated tool schemas come from the sqlc catalog, which
//     is built from this file.
//   - Tables carry column types, NOT NULL, defaults, and PRIMARY KEY/UNIQUE
//     constraints. Foreign keys, checks, indexes, and triggers are omitted:
//     they don't affect the query metadata sqlc produces.
//   - Views, materialized views, and continuous aggregates are emitted as
//     plain CREATE TABLE statements using their introspected columns — NOT
//     as CREATE VIEW with their defining SELECTs. See below.
//   - Functions and procedures are emitted verbatim (pg_get_functiondef),
//     so queries calling them resolve parameter names and result columns.
//     The schemas passed in must therefore have been fetched with
//     IncludeDefinitions=true.
//
// Why views are emitted as tables: given a faithful CREATE VIEW, sqlc
// ignores what Postgres knows about the view's columns and silently
// re-infers them from the SELECT text with its own, much weaker inference —
// e.g. a continuous aggregate's time_bucket column degrades to type "any",
// avg() columns come back NOT NULL, and a float8 expression column can come
// back as int, producing tool schemas that real results violate. The
// introspected columns are PostgreSQL's authoritative answer, and sqlc
// models tables and views both as relations, so queries validate against
// them identically. Long-term, sqlc's database-only analyzer (analyzerv2 /
// IntrospectSchema) may make the schema file — and this workaround —
// unnecessary.
//
// Statements are grouped by kind across all namespaces (schemas, then enums,
// then relations, then routines) so cross-schema references always resolve.
func FormatSchemaDDL(schemas []NamespacedSchema) string {
	var buf strings.Builder

	for _, ns := range schemas {
		if ns.Name == "public" {
			continue
		}
		fmt.Fprintf(&buf, "CREATE SCHEMA %s;\n\n", pgx.Identifier{ns.Name}.Sanitize())
	}

	for _, ns := range schemas {
		for _, enum := range ns.Enums {
			writeEnumDDL(&buf, ns.Name, enum)
		}
	}

	for _, ns := range schemas {
		for _, table := range ns.Tables {
			writeTableDDL(&buf, ns.Name, table.Name, tableColumnsDDL(table))
		}
		for _, view := range ns.Views {
			writeTableDDL(&buf, ns.Name, view.Name, viewColumnsDDL(view))
		}
		for _, mv := range ns.MaterializedViews {
			writeTableDDL(&buf, ns.Name, mv.Name, viewColumnsDDL(mv))
		}
	}

	for _, ns := range schemas {
		for _, fn := range ns.Functions {
			writeRoutineDDL(&buf, fn)
		}
		for _, proc := range ns.Procedures {
			writeRoutineDDL(&buf, proc)
		}
	}

	return buf.String()
}

func writeEnumDDL(buf *strings.Builder, schema string, enum EnumSchema) {
	vals := make([]string, len(enum.Values))
	for i, v := range enum.Values {
		vals[i] = quoteLiteral(v)
	}
	fmt.Fprintf(buf, "CREATE TYPE %s AS ENUM (%s);\n\n",
		pgx.Identifier{schema, enum.Name}.Sanitize(),
		strings.Join(vals, ", "),
	)
}

// tableColumnsDDL renders a table's column and constraint clauses, one per
// element. Only PRIMARY KEY and UNIQUE constraints are included; they inform
// nullability inference, while foreign keys and checks would not change
// anything sqlc reports.
func tableColumnsDDL(table TableSchema) []string {
	clauses := make([]string, 0, len(table.Columns)+len(table.Constraints))
	for _, col := range table.Columns {
		clause := fmt.Sprintf("%s %s", pgx.Identifier{col.Name}.Sanitize(), col.Type)
		if col.NotNull {
			clause += " NOT NULL"
		}
		if col.Default != "" {
			clause += " DEFAULT " + col.Default
		}
		clauses = append(clauses, clause)
	}
	for _, con := range table.Constraints {
		if len(con.Columns) == 0 {
			continue
		}
		var keyword string
		switch con.Type {
		case ConstraintPrimaryKey:
			keyword = "PRIMARY KEY"
		case ConstraintUnique:
			keyword = "UNIQUE"
		default:
			continue
		}
		cols := make([]string, len(con.Columns))
		for i, c := range con.Columns {
			cols[i] = pgx.Identifier{c}.Sanitize()
		}
		clauses = append(clauses, fmt.Sprintf("%s (%s)", keyword, strings.Join(cols, ", ")))
	}
	return clauses
}

// viewColumnsDDL renders a view's introspected columns as table column
// clauses (see FormatSchemaDDL for why views are emitted as tables). View
// column nullability is unknown, so every column is left nullable — the
// conservative choice for the generated tool schemas.
func viewColumnsDDL(view ViewSchema) []string {
	clauses := make([]string, len(view.Columns))
	for i, col := range view.Columns {
		clauses[i] = fmt.Sprintf("%s %s", pgx.Identifier{col.Name}.Sanitize(), col.Type)
	}
	return clauses
}

func writeTableDDL(buf *strings.Builder, schema, name string, clauses []string) {
	if len(clauses) == 0 {
		return
	}
	fmt.Fprintf(buf, "CREATE TABLE %s (\n    %s\n);\n\n",
		pgx.Identifier{schema, name}.Sanitize(),
		strings.Join(clauses, ",\n    "),
	)
}

func writeRoutineDDL(buf *strings.Builder, routine Routine) {
	if routine.Definition == "" {
		return
	}
	buf.WriteString(strings.TrimRight(routine.Definition, "\n;"))
	buf.WriteString(";\n\n")
}

// quoteLiteral renders a string as a single-quoted SQL literal.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
