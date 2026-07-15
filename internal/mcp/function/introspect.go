package function

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tools are defined by marking Postgres functions with an @mcp comment:
//
//	COMMENT ON FUNCTION get_pending_invoices IS
//	'@mcp
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
	// ModeExec runs for its side effects and returns no rows (RETURNS void).
	ModeExec Mode = "exec"
)

// Tool is the introspected metadata for one @mcp function.
type Tool struct {
	Schema      string
	Name        string
	Description string
	Mode        Mode
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
	// NullDefault marks arguments whose DEFAULT is a bare NULL constant —
	// the authoring convention for an argument that genuinely accepts null.
	// Only these arguments admit null in the tool's input schema.
	NullDefault bool
	Type        TypeInfo
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

// marker is the tag that exposes a function as an MCP tool. It must be the
// first non-blank line of the function's comment, alone on its line.
const marker = "@mcp"

// parseMarkerComment reports whether comment carries the @mcp marker, and
// returns the remaining lines as the tool description.
func parseMarkerComment(comment string) (string, bool) {
	trimmed := strings.TrimLeft(comment, " \t\n")
	first, rest, _ := strings.Cut(trimmed, "\n")
	if strings.TrimSpace(first) != marker {
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
	ArgModes    []string `db:"arg_modes"` // nil when every argument is IN
	ArgNames    []string `db:"arg_names"` // nil when no argument is named
	ArgTypes    []int64  `db:"arg_types"`
	// ArgDefaults holds each argument's deparsed DEFAULT expression, nil for
	// arguments without one; the slice is aligned with ArgTypes.
	ArgDefaults []*string `db:"arg_defaults"`
	// RetColumns holds the attributes of a composite return type (a table
	// row type or CREATE TYPE ... AS); nil for other return types.
	RetColumns []retColumn `db:"ret_columns"`
}

// retColumn is one attribute of a composite return type, aggregated into the
// ret_columns JSON column of functionsQuery.
type retColumn struct {
	Name string `json:"name"`
	Type int64  `json:"type"`
}

// functionsQuery selects every routine — plain functions, but also
// procedures, aggregates, and window functions, so buildTool can reject the
// unsupported kinds loudly — whose comment starts with the @mcp marker (the
// marker is re-validated precisely in Go).
// proargtypes is an oidvector, which has no direct array cast, so it is
// round-tripped through its space-separated text form. proallargtypes is
// only set when the function has OUT/INOUT/TABLE/VARIADIC arguments, and
// then covers all arguments. Argument defaults are deparsed one by one with
// pg_get_function_arg_default (which returns NULL for arguments without a
// default), producing an array aligned with arg_types. For functions
// returning a composite type (a
// table row type or CREATE TYPE ... AS), the composite's attributes ride
// along as JSON in ret_columns.
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
    p.proargmodes::text[] AS arg_modes,
    p.proargnames AS arg_names,
    COALESCE(
        p.proallargtypes::int8[],
        string_to_array(p.proargtypes::text, ' ')::int8[]
    ) AS arg_types,
    (
        SELECT array_agg(pg_catalog.pg_get_function_arg_default(p.oid, i) ORDER BY i)
        FROM pg_catalog.generate_series(
            1,
            COALESCE(pg_catalog.array_length(p.proallargtypes, 1), p.pronargs::int4)
        ) AS i
    ) AS arg_defaults,
    (
        SELECT json_agg(
            json_build_object('name', a.attname, 'type', a.atttypid::int8)
            ORDER BY a.attnum
        )
        FROM pg_catalog.pg_attribute a
        WHERE a.attrelid = rt.typrelid AND a.attnum > 0 AND NOT a.attisdropped
    ) AS ret_columns
FROM pg_catalog.pg_proc p
JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
JOIN pg_catalog.pg_type rt ON rt.oid = p.prorettype
JOIN pg_catalog.pg_description d
    ON d.objoid = p.oid
    AND d.classoid = 'pg_catalog.pg_proc'::regclass
    AND d.objsubid = 0
WHERE d.description ~ '^\s*@mcp'
ORDER BY n.nspname, p.proname`

// Introspect reads every @mcp-marked function from the database catalog and
// returns their tool metadata. Functions that can't be exposed — overloaded
// @mcp names, unsupported argument or return types — are skipped with a
// logged warning, never an error: one exotic function must not take down
// the rest of the tool surface.
//
// The whole pass costs a handful of queries regardless of how many functions
// or types are involved: one for the functions (composite return columns
// included) and one or two batched type-info loads (see typeResolver.preload).
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
		if desc, ok := parseMarkerComment(row.Comment); ok {
			row.Comment = desc
			marked = append(marked, row)
		}
	}

	// An overloaded name can't become a tool: the tool's input schema and
	// call can't distinguish the overloads. Skip every @mcp function whose
	// (schema, name) appears more than once.
	counts := make(map[string]int, len(marked))
	for _, row := range marked {
		counts[row.SchemaName+"."+row.Name]++
	}

	// Preload the catalog rows for every type the marked functions
	// reference, so building the tools below never queries the database.
	resolver := newTypeResolver(pool)
	var oids []int64
	for _, row := range marked {
		oids = append(oids, row.ArgTypes...)
		oids = append(oids, row.RetType)
		for _, col := range row.RetColumns {
			oids = append(oids, col.Type)
		}
	}
	if err := resolver.preload(ctx, oids); err != nil {
		return nil, err
	}

	tools := make([]Tool, 0, len(marked))
	for _, row := range marked {
		if counts[row.SchemaName+"."+row.Name] > 1 {
			logger.Warn("Skipping @mcp function: overloaded functions cannot be exposed as tools",
				slog.String("function", row.SchemaName+"."+row.Name),
			)
			continue
		}
		tool, err := buildTool(resolver, row)
		if err != nil {
			logger.Warn("Skipping @mcp function",
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
func buildTool(resolver *typeResolver, row functionRow) (Tool, error) {
	// Only plain functions become tools. Procedures are deliberately not
	// supported: a void-returning function covers the same ground, and each
	// tool call runs as its own transaction anyway. Aggregates and window
	// functions compute over rows the caller must supply in a query, which a
	// standalone tool call has none of. All are still selected by
	// functionsQuery so a marked routine is skipped loudly rather than
	// silently ignored.
	switch row.Kind {
	case "p":
		return Tool{}, fmt.Errorf("procedures are not supported; use a function returning void instead")
	case "a":
		return Tool{}, fmt.Errorf("aggregate functions are not supported")
	case "w":
		return Tool{}, fmt.Errorf("window functions are not supported")
	}

	params, err := inputParams(resolver, row)
	if err != nil {
		return Tool{}, err
	}

	named := !slices.ContainsFunc(params, func(p Param) bool {
		return p.ArgName == ""
	})

	tool := Tool{
		Schema:      row.SchemaName,
		Name:        row.Name,
		Description: row.Comment,
		ReadOnly:    row.Volatility == "i" || row.Volatility == "s",
		Params:      params,
		Named:       named,
	}

	// Determine the result shape.
	if row.RetTypeName == "void" {
		tool.Mode = ModeExec
		return tool, nil
	}

	tool.Mode = ModeOne
	if row.ReturnsSet {
		tool.Mode = ModeMany
	}
	cols, err := resultColumns(resolver, row)
	if err != nil {
		return Tool{}, err
	}
	tool.Columns = cols

	return tool, nil
}

// argMode returns the mode of the function's i'th argument. proargmodes is
// only set when the function has non-IN arguments, and then covers all
// arguments in declaration order.
func argMode(row functionRow, i int) string {
	if row.ArgModes != nil {
		return row.ArgModes[i]
	}
	return "i" // IN
}

// argName returns the name of the function's i'th argument, "" when unnamed.
func argName(row functionRow, i int) string {
	if row.ArgNames != nil {
		return row.ArgNames[i]
	}
	return ""
}

// argDefault returns the deparsed DEFAULT expression of the function's i'th
// argument, and whether it has one.
func argDefault(row functionRow, i int) (string, bool) {
	if i < len(row.ArgDefaults) && row.ArgDefaults[i] != nil {
		return *row.ArgDefaults[i], true
	}
	return "", false
}

// isNullDefault reports whether a deparsed argument default is a bare NULL
// constant, which pg_get_function_arg_default renders as NULL with an
// optional type annotation ("NULL::integer"). Expressions that merely
// evaluate to null (e.g. NULLIF(1, 1)) deliberately don't match.
func isNullDefault(def string) bool {
	return def == "NULL" || strings.HasPrefix(def, "NULL::")
}

// inputParams returns the function's input parameters (IN and INOUT
// arguments) in declaration order, and validates that every argument mode
// is supported.
func inputParams(resolver *typeResolver, row functionRow) ([]Param, error) {
	// An unnamed argument falls back to a param_<N> name; skip any N already
	// taken by a real, explicitly-named argument (e.g. a second argument
	// named "param_1") so the fallback can never collide with it.
	usedNames := make(map[string]bool, len(row.ArgTypes))
	for i := range row.ArgTypes {
		if name := argName(row, i); name != "" {
			usedNames[name] = true
		}
	}
	nextFallback := 1

	var params []Param

	for i, typeOID := range row.ArgTypes {
		switch mode := argMode(row, i); mode {
		case "i", "b": // IN, INOUT
		case "o", "t": // OUT, TABLE: result columns, not inputs
			continue
		case "v":
			return nil, fmt.Errorf("VARIADIC arguments are not supported")
		default:
			return nil, fmt.Errorf("unsupported argument mode %q", mode)
		}

		typ, err := resolver.resolve(typeOID)
		if err != nil {
			return nil, err
		}

		name := argName(row, i)
		paramName := name
		if paramName == "" {
			for {
				candidate := fmt.Sprintf("param_%d", nextFallback)
				nextFallback++
				if !usedNames[candidate] {
					paramName = candidate
					usedNames[candidate] = true
					break
				}
			}
		}
		def, hasDefault := argDefault(row, i)
		params = append(params, Param{
			Name:        paramName,
			ArgName:     name,
			HasDefault:  hasDefault,
			NullDefault: hasDefault && isNullDefault(def),
			Type:        typ,
		})
	}

	return params, nil
}

// outputColumns returns the result columns declared by the function's OUT,
// INOUT, and TABLE arguments, in declaration order. Empty when the function
// declares none — its result shape then comes from the return type instead
// (see resultColumns).
func outputColumns(resolver *typeResolver, row functionRow) ([]Column, error) {
	var cols []Column

	for i, typeOID := range row.ArgTypes {
		switch argMode(row, i) {
		case "o", "b", "t": // OUT, INOUT, TABLE
		default:
			continue
		}

		typ, err := resolver.resolve(typeOID)
		if err != nil {
			return nil, err
		}

		name := argName(row, i)
		if name == "" {
			// PostgreSQL names unnamed output columns by their position in
			// the output column list.
			name = fmt.Sprintf("column%d", len(cols)+1)
		}
		cols = append(cols, Column{Name: name, Type: typ})
	}

	return cols, nil
}

// resultColumns determines the result columns for a function that returns
// rows, in order of preference: OUT/INOUT/TABLE arguments define the shape
// when present; a composite return type contributes its attributes; any
// other type is a single column named after the function.
func resultColumns(resolver *typeResolver, row functionRow) ([]Column, error) {
	outCols, err := outputColumns(resolver, row)
	if err != nil {
		return nil, err
	}
	if len(outCols) > 0 {
		return outCols, nil
	}

	if len(row.RetColumns) > 0 {
		// Composite return type (a table row type or CREATE TYPE ... AS).
		cols := make([]Column, len(row.RetColumns))
		for i, attr := range row.RetColumns {
			typ, err := resolver.resolve(attr.Type)
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
	typ, err := resolver.resolve(row.RetType)
	if err != nil {
		return nil, err
	}
	return []Column{{Name: row.Name, Type: typ}}, nil
}
