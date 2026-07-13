package function

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tools are defined by marking Postgres functions with an @api comment:
//
//	COMMENT ON FUNCTION get_pending_invoices IS
//	'@api
//	Returns unpaid invoices for a customer, ordered by due date.';
//
// Introspect reads every marked function straight from the catalog — names,
// parameters, defaults, return shape, and volatility all come from pg_proc,
// so there is nothing to parse or validate beyond the marker itself.

// Mode describes how a tool returns results.
type Mode string

const (
	// ModeOne returns a single result row (RETURNS <scalar or composite>).
	ModeOne Mode = "one"
	// ModeMany returns a set of rows (RETURNS SETOF / RETURNS TABLE).
	ModeMany Mode = "many"
	// ModeExec runs for its side effects and returns no rows (RETURNS void,
	// or a procedure).
	ModeExec Mode = "exec"
)

// Tool is the introspected metadata for one @api function.
type Tool struct {
	Schema      string
	Name        string
	Description string
	Mode        Mode
	// IsProcedure marks a procedure (invoked with CALL) rather than a
	// function (invoked with SELECT).
	IsProcedure bool
	// ReadOnly reports whether the function is marked IMMUTABLE or STABLE.
	// Unlike a plan-based classification, this is the author's own
	// declaration, which is also what the planner trusts.
	ReadOnly bool
	Params   []Param
	// Columns are the result columns; empty in ModeExec.
	Columns []Column
	// Named reports whether every input argument has a name. Named
	// arguments allow calls that omit any subset of defaulted arguments
	// (named notation); without names, omitted defaults must form a
	// trailing suffix of the positional argument list.
	Named bool
}

// Param is one input argument of a tool's function.
type Param struct {
	// Name is the parameter's name in the tool's input schema: the
	// function's argument name, or param_<N> when the argument is unnamed.
	Name string
	// ArgName is the actual Postgres argument name ("" when unnamed), used
	// for named-notation calls.
	ArgName string
	// HasDefault marks arguments with a DEFAULT, which are optional in the
	// tool's input schema.
	HasDefault bool
	Type       TypeInfo
}

// Column is one result column of a tool's function.
type Column struct {
	Name string
	Type TypeInfo
}

// TypeInfo describes a Postgres type as needed for JSON Schema generation
// and result scanning. Domains are resolved to their base type.
type TypeInfo struct {
	// Name is the type's SQL name (from format_type), e.g. "integer",
	// "timestamp with time zone", "mood". For arrays it names the element
	// type.
	Name    string
	IsArray bool
	// EnumVals holds the enum labels when the (element) type is an enum.
	EnumVals []string
}

// apiMarker matches the @api marker that must be the first non-blank line of
// the function's comment. An optional parenthesized group list — reserved
// for scoping tools to named agent groups — is accepted and currently
// ignored.
var apiMarker = regexp.MustCompile(`^@api(\(([^)]*)\))?$`)

// parseAPIComment reports whether comment carries the @api marker, and
// returns the remaining lines as the tool description.
func parseAPIComment(comment string) (string, bool) {
	trimmed := strings.TrimLeft(comment, " \t\n")
	first, rest, _ := strings.Cut(trimmed, "\n")
	if !apiMarker.MatchString(strings.TrimSpace(first)) {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// functionRow is the raw catalog row for one commented function.
type functionRow struct {
	SchemaName  string   `db:"schema_name"`
	Name        string   `db:"function_name"`
	Comment     string   `db:"comment"`
	Kind        string   `db:"kind"`
	Volatility  string   `db:"volatility"`
	ReturnsSet  bool     `db:"returns_set"`
	RetType     int64    `db:"rettype"`
	RetTypeName string   `db:"rettype_name"`
	RetTypeType string   `db:"rettype_type"`
	RetTypeRel  int64    `db:"rettype_relid"`
	NumDefaults int      `db:"num_defaults"`
	ArgModes    []string `db:"arg_modes"` // nil when every argument is IN
	ArgNames    []string `db:"arg_names"` // nil when no argument is named
	ArgTypes    []int64  `db:"arg_types"`
}

// functionsQuery selects every function or procedure whose comment starts
// with the @api marker (the marker is re-validated precisely in Go).
// proargtypes is an oidvector, which has no direct array cast, so it is
// round-tripped through its space-separated text form. proallargtypes is
// only set when the function has OUT/INOUT/TABLE/VARIADIC arguments, and
// then covers all arguments.
const functionsQuery = `
SELECT
    n.nspname AS schema_name,
    p.proname AS function_name,
    d.description AS comment,
    p.prokind::text AS kind,
    p.provolatile::text AS volatility,
    p.proretset AS returns_set,
    p.prorettype::int8 AS rettype,
    pg_catalog.format_type(p.prorettype, NULL) AS rettype_name,
    rt.typtype::text AS rettype_type,
    rt.typrelid::int8 AS rettype_relid,
    p.pronargdefaults AS num_defaults,
    p.proargmodes::text[] AS arg_modes,
    p.proargnames AS arg_names,
    COALESCE(
        p.proallargtypes::int8[],
        string_to_array(p.proargtypes::text, ' ')::int8[]
    ) AS arg_types
FROM pg_catalog.pg_proc p
JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
JOIN pg_catalog.pg_type rt ON rt.oid = p.prorettype
JOIN pg_catalog.pg_description d
    ON d.objoid = p.oid
    AND d.classoid = 'pg_catalog.pg_proc'::regclass
    AND d.objsubid = 0
WHERE p.prokind IN ('f', 'p')
  AND d.description ~ '^\s*@api'
ORDER BY n.nspname, p.proname`

// compositeColumnsQuery returns the columns of a composite type (a table
// row type or CREATE TYPE ... AS), for functions returning SETOF <table> or
// a composite.
const compositeColumnsQuery = `
SELECT a.attname, a.atttypid::int8
FROM pg_catalog.pg_attribute a
WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`

// Introspect reads every @api-marked function from the database catalog and
// returns their tool metadata. Functions that can't be exposed — overloaded
// @api names, unsupported argument or return types — are skipped with a
// logged warning, never an error: one exotic function must not take down
// the rest of the tool surface.
func Introspect(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) ([]Tool, error) {
	rows, err := pool.Query(ctx, functionsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect functions: %w", err)
	}
	fnRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[functionRow])
	if err != nil {
		return nil, fmt.Errorf("failed to introspect functions: %w", err)
	}

	// Re-validate the marker precisely and drop non-matches.
	marked := fnRows[:0]
	for _, row := range fnRows {
		if desc, ok := parseAPIComment(row.Comment); ok {
			row.Comment = desc
			marked = append(marked, row)
		}
	}

	// An overloaded name can't become a tool: the tool's input schema and
	// call can't distinguish the overloads. Skip every @api function whose
	// (schema, name) appears more than once.
	counts := make(map[string]int, len(marked))
	for _, row := range marked {
		counts[row.SchemaName+"."+row.Name]++
	}

	resolver := newTypeResolver(pool)

	tools := make([]Tool, 0, len(marked))
	for _, row := range marked {
		if counts[row.SchemaName+"."+row.Name] > 1 {
			logger.Warn("Skipping @api function: overloaded functions cannot be exposed as tools",
				slog.String("function", row.SchemaName+"."+row.Name),
			)
			continue
		}
		tool, err := buildTool(ctx, resolver, row)
		if err != nil {
			logger.Warn("Skipping @api function",
				slog.String("function", row.SchemaName+"."+row.Name),
				slog.Any("error", err),
			)
			continue
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

// buildTool converts one catalog row into tool metadata.
func buildTool(ctx context.Context, resolver *typeResolver, row functionRow) (Tool, error) {
	params, outCols, err := splitArgs(ctx, resolver, row)
	if err != nil {
		return Tool{}, err
	}

	named := true
	for _, p := range params {
		if p.ArgName == "" {
			named = false
			break
		}
	}

	tool := Tool{
		Schema:      row.SchemaName,
		Name:        row.Name,
		Description: row.Comment,
		IsProcedure: row.Kind == "p",
		ReadOnly:    row.Volatility == "i" || row.Volatility == "s",
		Params:      params,
		Named:       named,
	}

	// Determine the result shape.
	switch {
	case row.Kind == "p":
		// Procedures run via CALL for their side effects. Procedures with
		// OUT/INOUT arguments return a result row, which is not supported
		// yet.
		if len(outCols) > 0 {
			return Tool{}, fmt.Errorf("procedures with OUT arguments are not supported")
		}
		tool.Mode = ModeExec
	case row.RetTypeName == "void":
		tool.Mode = ModeExec
	default:
		tool.Mode = ModeOne
		if row.ReturnsSet {
			tool.Mode = ModeMany
		}
		cols, err := resultColumns(ctx, resolver, row, outCols)
		if err != nil {
			return Tool{}, err
		}
		tool.Columns = cols
	}

	return tool, nil
}

// splitArgs partitions the function's arguments into input parameters and
// output columns (OUT/INOUT/TABLE arguments) using the pg_proc argument
// arrays, which cover all arguments in declaration order.
func splitArgs(ctx context.Context, resolver *typeResolver, row functionRow) ([]Param, []Column, error) {
	var params []Param
	var outCols []Column

	for i, typeOID := range row.ArgTypes {
		mode := "i"
		if row.ArgModes != nil {
			mode = row.ArgModes[i]
		}
		name := ""
		if row.ArgNames != nil {
			name = row.ArgNames[i]
		}

		typ, err := resolver.resolve(ctx, typeOID)
		if err != nil {
			return nil, nil, err
		}

		switch mode {
		case "i", "b": // IN, INOUT
			paramName := name
			if paramName == "" {
				paramName = fmt.Sprintf("param_%d", len(params)+1)
			}
			params = append(params, Param{
				Name:    paramName,
				ArgName: name,
				Type:    typ,
			})
		case "o", "t": // OUT, TABLE
			if name == "" {
				name = fmt.Sprintf("column%d", len(outCols)+1)
			}
			outCols = append(outCols, Column{Name: name, Type: typ})
		case "v":
			return nil, nil, fmt.Errorf("VARIADIC arguments are not supported")
		default:
			return nil, nil, fmt.Errorf("unsupported argument mode %q", mode)
		}

		if mode == "b" { // INOUT arguments are both a parameter and a column
			colName := name
			if colName == "" {
				colName = fmt.Sprintf("column%d", len(outCols)+1)
			}
			outCols = append(outCols, Column{Name: colName, Type: typ})
		}
	}

	// Defaults always attach to the trailing input arguments.
	for i := len(params) - row.NumDefaults; i < len(params); i++ {
		if i >= 0 {
			params[i].HasDefault = true
		}
	}

	return params, outCols, nil
}

// resultColumns determines the result columns for a function that returns
// rows, in order of preference: OUT/INOUT/TABLE arguments define the shape
// when present; a composite return type contributes its attributes; any
// other type is a single column named after the function.
func resultColumns(ctx context.Context, resolver *typeResolver, row functionRow, outCols []Column) ([]Column, error) {
	if len(outCols) > 0 {
		return outCols, nil
	}

	if row.RetTypeRel != 0 {
		// Composite return type (a table row type or CREATE TYPE ... AS).
		rows, err := resolver.pool.Query(ctx, compositeColumnsQuery, row.RetTypeRel)
		if err != nil {
			return nil, fmt.Errorf("failed to introspect composite return type: %w", err)
		}
		attrs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct {
			Name    string
			TypeOID int64
		}])
		if err != nil {
			return nil, fmt.Errorf("failed to introspect composite return type: %w", err)
		}
		cols := make([]Column, len(attrs))
		for i, attr := range attrs {
			typ, err := resolver.resolve(ctx, attr.TypeOID)
			if err != nil {
				return nil, err
			}
			cols[i] = Column{Name: attr.Name, Type: typ}
		}
		return cols, nil
	}

	if row.RetTypeType == "p" {
		// Pseudo-type return (record without OUT arguments, polymorphic
		// types, etc.): the result shape can't be determined statically.
		return nil, fmt.Errorf("unsupported return type %q", row.RetTypeName)
	}

	// Scalar return: a single column named after the function, which is how
	// PostgreSQL itself names it in SELECT * FROM f(...).
	typ, err := resolver.resolve(ctx, row.RetType)
	if err != nil {
		return nil, err
	}
	return []Column{{Name: row.Name, Type: typ}}, nil
}
