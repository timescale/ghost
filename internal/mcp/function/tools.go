package function

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/util"
)

// buildMCPTool constructs the MCP tool definition and handler for a single
// @mcp function. toolName is the full (database-prefixed) tool name; the
// tool's schemas and annotations come from the function's introspected
// metadata, and the handler calls the function through pool.
func buildMCPTool(toolName string, tool Tool, pool *pgxpool.Pool) (*mcp.Tool, mcp.ToolHandler) {
	def := &mcp.Tool{
		Name:         toolName,
		Description:  tool.Description,
		InputSchema:  buildInputSchema(tool),
		OutputSchema: buildOutputSchema(tool),
		Annotations:  toolAnnotations(tool),
	}

	// The Go types the result columns scan into are fixed by the function's
	// introspected metadata, so compute them once here.
	types := scanTypes(tool.Columns)

	handler := func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, fmt.Errorf("failed to parse arguments: %w", err)
			}
		}

		return handleToolCall(ctx, pool, tool, types, args), nil
	}

	return def, handler
}

// toolAnnotations builds the annotation hints for a tool. Every tool
// operates on a single closed database, so the world is closed. The
// read-only hint comes from the function's own volatility declaration
// (IMMUTABLE/STABLE); a VOLATILE function may write, and whether its writes
// are destructive can't be determined, so the destructive hint is left at
// its conservative default.
func toolAnnotations(tool Tool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:  tool.ReadOnly,
		OpenWorldHint: new(false),
	}
}

// buildInputSchema builds the tool's input schema from the function's
// arguments. Arguments without a DEFAULT are required. Every argument
// admits null in addition to its base type, since PostgreSQL function
// arguments can always be passed NULL explicitly.
func buildInputSchema(tool Tool) *jsonschema.Schema {
	properties := map[string]*jsonschema.Schema{}
	var required []string

	for _, param := range tool.Params {
		properties[param.Name] = allowNull(typeSchema(param.Type))
		if !param.HasDefault {
			required = append(required, param.Name)
		}
	}

	return &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

// buildOutputSchema builds the tool's output schema from the function's
// result columns. Column nullability is unknowable from the catalog, so
// every column admits null — the conservative choice.
func buildOutputSchema(tool Tool) *jsonschema.Schema {
	if tool.Mode == ModeExec {
		return &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"success": {
					Type:        "boolean",
					Description: "Whether the call completed successfully",
				},
			},
			Required: []string{"success"},
		}
	}

	properties := map[string]*jsonschema.Schema{}
	for _, col := range tool.Columns {
		properties[col.Name] = allowNull(typeSchema(col.Type))
	}

	itemSchema := &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
	}

	if tool.Mode == ModeOne {
		return itemSchema
	}

	// ModeMany: wrap the array in an object with a 'results' field to
	// satisfy the MCP SDK's requirement that output schemas have type
	// "object" at the root.
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"results": {
				Type:  "array",
				Items: itemSchema,
			},
		},
		Required: []string{"results"},
	}
}

// typeSchema maps a Postgres type to the JSON Schema describing a single
// value of that type, wrapping array types in one array level (with
// nullable elements — PostgreSQL array elements can always be NULL).
func typeSchema(typ TypeInfo) *jsonschema.Schema {
	schema := scalarTypeSchema(typ)
	if typ.IsArray {
		schema = &jsonschema.Schema{
			Type:  "array",
			Items: allowNull(schema),
		}
	}
	return schema
}

// scalarTypeSchema maps a Postgres type name to the JSON Schema describing
// how a single non-null, non-array value of that type appears in tool input
// and output.
func scalarTypeSchema(typ TypeInfo) *jsonschema.Schema {
	// Enum types list their values from the catalog.
	if len(typ.EnumVals) > 0 {
		return &jsonschema.Schema{
			Type: "string",
			Enum: util.AnySlice(typ.EnumVals),
		}
	}

	switch typ.Name {
	case "smallint", "int2", "smallserial", "serial2":
		return &jsonschema.Schema{
			Type:    "integer",
			Minimum: new(float64(math.MinInt16)),
			Maximum: new(float64(math.MaxInt16)),
		}
	case "integer", "int", "int4", "serial", "serial4":
		return &jsonschema.Schema{
			Type:    "integer",
			Minimum: new(float64(math.MinInt32)),
			Maximum: new(float64(math.MaxInt32)),
		}
	case "bigint", "int8", "bigserial", "serial8":
		// float64 cannot represent MaxInt64 exactly, so the advertised
		// maximum rounds up to 2^63; the bounds are magnitude hints.
		return &jsonschema.Schema{
			Type:    "integer",
			Minimum: new(float64(math.MinInt64)),
			Maximum: new(float64(math.MaxInt64)),
		}
	case "oid":
		return &jsonschema.Schema{
			Type:    "integer",
			Minimum: new(float64(0)),
			Maximum: new(float64(math.MaxUint32)),
		}
	case "real", "float4", "double precision", "float8", "float":
		return &jsonschema.Schema{Type: "number"}
	case "numeric", "decimal":
		return &jsonschema.Schema{
			Type:        "number",
			Description: "Arbitrary-precision number",
		}
	case "boolean", "bool":
		return &jsonschema.Schema{Type: "boolean"}
	case "json", "jsonb":
		// Any JSON value is allowed, so no type constraint.
		return &jsonschema.Schema{Description: "Arbitrary JSON value (object, array, string, number, boolean, or null)"}
	case "uuid":
		return &jsonschema.Schema{
			Type:   "string",
			Format: "uuid",
		}
	case "date":
		return &jsonschema.Schema{
			Type:   "string",
			Format: "date",
		}
	case "timestamp", "timestamp without time zone":
		// Not format: date-time, which requires a UTC offset that a
		// zoneless timestamp does not carry.
		return &jsonschema.Schema{
			Type:        "string",
			Description: "Timestamp without time zone (ISO 8601, e.g. 2026-07-02T11:25:16)",
		}
	case "timestamptz", "timestamp with time zone":
		return &jsonschema.Schema{
			Type:   "string",
			Format: "date-time",
		}
	case "time", "time without time zone", "timetz", "time with time zone":
		return &jsonschema.Schema{
			Type:        "string",
			Description: "Time of day (HH:MM:SS[.ffffff])",
		}
	case "interval":
		return &jsonschema.Schema{
			Type:        "string",
			Description: "PostgreSQL interval (e.g. '1 year 2 mons 3 days 04:05:06')",
		}
	case "bytea":
		return &jsonschema.Schema{
			Type:            "string",
			ContentEncoding: "base64",
			Description:     "Binary data, base64-encoded",
		}
	case "inet":
		return &jsonschema.Schema{
			Type:        "string",
			Description: "IP address, optionally with a network prefix (e.g. 192.168.0.1 or 2001:db8::/64)",
		}
	case "cidr":
		return &jsonschema.Schema{
			Type:        "string",
			Description: "Network address in CIDR notation",
		}
	case "macaddr", "macaddr8":
		return &jsonschema.Schema{
			Type:        "string",
			Description: "MAC address",
		}
	case "money":
		return &jsonschema.Schema{
			Type:        "string",
			Description: "Currency amount as formatted by the database locale",
		}
	default:
		// Everything else — text types, user-defined types, ranges,
		// composites — renders through its canonical Postgres text form.
		return &jsonschema.Schema{Type: "string"}
	}
}

// allowNull extends a schema to accept null in addition to its base type.
// Every schema it receives is freshly built, so Type is either a single
// string or empty (json/jsonb), never already a union.
func allowNull(schema *jsonschema.Schema) *jsonschema.Schema {
	if schema.Type == "" {
		// No type constraint (e.g. json/jsonb) already admits null.
		return schema
	}
	schema.Types = []string{schema.Type, "null"}
	schema.Type = ""

	// An enum constraint validates independently of the type, so null must
	// be one of the allowed values.
	if schema.Enum != nil {
		schema.Enum = append(schema.Enum, nil)
	}

	return schema
}

// convertParamValue converts a JSON argument into the value pgx should send
// for the parameter's PostgreSQL type. Most JSON values pass through as-is,
// but bytea parameters arrive as base64 strings (as their schema advertises)
// and must be decoded; sending the string unchanged would store the base64
// text itself.
func convertParamValue(typ TypeInfo, val any) (any, error) {
	if typ.Name != "bytea" {
		return val, nil
	}
	return decodeBase64Values(val)
}

// decodeBase64Values decodes a base64 string, or each string in a (possibly
// nested) array of them, for bytea parameters.
func decodeBase64Values(val any) (any, error) {
	switch v := val.(type) {
	case nil:
		return nil, nil
	case string:
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, fmt.Errorf("expected base64-encoded binary data: %w", err)
		}
		return decoded, nil
	case []any:
		out := make([]any, len(v))
		for i, elem := range v {
			decoded, err := decodeBase64Values(elem)
			if err != nil {
				return nil, err
			}
			out[i] = decoded
		}
		return out, nil
	default:
		return val, nil
	}
}

// errorResult creates a CallToolResult for an error condition
func errorResult(format string, args ...any) *mcp.CallToolResult {
	errMsg := fmt.Sprintf(format, args...)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: errMsg},
		},
		IsError: true,
	}
}

// successResult creates a CallToolResult for a successful result
func successResult(result any) *mcp.CallToolResult {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return errorResult("failed to marshal result to JSON: %v", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(resultJSON)},
		},
		StructuredContent: result,
	}
}

// buildCall renders the SQL invocation for one tool call and the bound
// argument values, honoring argument defaults: optional arguments the
// caller omitted are left out of the call so PostgreSQL applies their
// defaults. When every argument is provided the call uses positional
// notation; otherwise named notation ("arg" => $n) skips the omitted
// arguments, which requires the function's arguments to be named — for a
// function with unnamed arguments, the omitted optionals must form a
// trailing suffix of the argument list.
func buildCall(tool Tool, input map[string]any) (string, []any, error) {
	type callArg struct {
		param Param
		value any
	}
	var provided []callArg
	var omitted []string

	for _, param := range tool.Params {
		val, ok := input[param.Name]
		if !ok {
			if !param.HasDefault {
				return "", nil, fmt.Errorf("missing required parameter: %s", param.Name)
			}
			omitted = append(omitted, param.Name)
			continue
		}
		val, err := convertParamValue(param.Type, val)
		if err != nil {
			return "", nil, fmt.Errorf("invalid value for parameter %s: %w", param.Name, err)
		}
		if len(omitted) > 0 && !tool.Named {
			return "", nil, fmt.Errorf("parameter %s requires %s to also be provided (the function's arguments are unnamed, so defaults can only be omitted from the end)", param.Name, strings.Join(omitted, ", "))
		}
		provided = append(provided, callArg{param: param, value: val})
	}

	parts := make([]string, len(provided))
	values := make([]any, len(provided))
	useNamed := tool.Named && len(omitted) > 0
	for i, arg := range provided {
		placeholder := fmt.Sprintf("$%d", i+1)
		if useNamed {
			parts[i] = pgx.Identifier{arg.param.ArgName}.Sanitize() + " => " + placeholder
		} else {
			parts[i] = placeholder
		}
		values[i] = arg.value
	}

	fnName := pgx.Identifier{tool.Schema, tool.Name}.Sanitize()
	argList := strings.Join(parts, ", ")

	var sql string
	if tool.Mode == ModeExec {
		// A void-returning function is invoked bare: there are no result
		// columns to expand.
		sql = fmt.Sprintf("SELECT %s(%s)", fnName, argList)
	} else {
		// SELECT * FROM expands composite and scalar results alike into the
		// introspected result columns.
		sql = fmt.Sprintf("SELECT * FROM %s(%s)", fnName, argList)
	}

	return sql, values, nil
}

func handleToolCall(ctx context.Context, pool *pgxpool.Pool, tool Tool, types []reflect.Type, input map[string]any) *mcp.CallToolResult {
	sql, args, err := buildCall(tool, input)
	if err != nil {
		return errorResult("%v", err)
	}

	switch tool.Mode {
	case ModeExec:
		return executeExec(ctx, pool, sql, args)
	case ModeOne:
		return executeOne(ctx, pool, sql, types, args)
	case ModeMany:
		return executeMany(ctx, pool, sql, types, args)
	default:
		return errorResult("unknown tool mode: %s", tool.Mode)
	}
}

func executeExec(ctx context.Context, pool *pgxpool.Pool, sql string, args []any) *mcp.CallToolResult {
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		return errorResult("call failed: %v", err)
	}

	result := map[string]any{
		"success": true,
	}

	return successResult(result)
}

func executeOne(ctx context.Context, pool *pgxpool.Pool, sql string, types []reflect.Type, args []any) *mcp.CallToolResult {
	rows, err := pool.Query(ctx, sql, slices.Concat([]any{textResults}, args)...)
	if err != nil {
		return errorResult("call failed: %v", err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()

	if !rows.Next() {
		return errorResult("no rows returned")
	}

	dests := scanDests(types)
	if err := rows.Scan(dests...); err != nil {
		return errorResult("failed to scan row: %v", err)
	}

	result := rowToMap(fieldDescs, dests)

	if rows.Next() {
		return errorResult("expected one row, got multiple")
	}

	if err := rows.Err(); err != nil {
		return errorResult("row iteration error: %v", err)
	}

	return successResult(result)
}

func executeMany(ctx context.Context, pool *pgxpool.Pool, sql string, types []reflect.Type, args []any) *mcp.CallToolResult {
	rows, err := pool.Query(ctx, sql, slices.Concat([]any{textResults}, args)...)
	if err != nil {
		return errorResult("call failed: %v", err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()

	var results []map[string]any
	for rows.Next() {
		dests := scanDests(types)
		if err := rows.Scan(dests...); err != nil {
			return errorResult("failed to scan row: %v", err)
		}

		results = append(results, rowToMap(fieldDescs, dests))
	}

	if err := rows.Err(); err != nil {
		return errorResult("row iteration error: %v", err)
	}

	// Wrap results in an object to match the output schema
	result := map[string]any{
		"results": results,
	}

	return successResult(result)
}
