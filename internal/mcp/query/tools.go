package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BuildQueryTool constructs the MCP tool definition and handler for a single
// query. toolName is the full (database-prefixed) tool name; the tool's
// schemas and annotations come from the query's sqlc metadata and EXPLAIN
// classification, and the handler executes the query through pool.
func BuildQueryTool(toolName string, query Query, meta *QueryMetadata, pool *pgxpool.Pool) (*mcp.Tool, mcp.ToolHandler) {
	// Build input schema from params and output schema from columns.
	inputSchema := buildInputSchema(query, meta)
	outputSchema := buildOutputSchema(query, meta)

	// Build the description from the query's doc comments. The backing SQL text
	// is an internal implementation detail and is intentionally omitted.
	description := ""
	for _, comment := range query.Comments {
		if description != "" {
			description += "\n"
		}
		description += strings.TrimSpace(comment)
	}

	tool := &mcp.Tool{
		Name:         toolName,
		Description:  description,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Annotations:  queryToolAnnotations(query),
	}

	// The Go types the result columns scan into are fixed by the query's
	// column metadata, so compute them once here.
	types := scanTypes(query.Columns)

	handler := func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments from request
		var args map[string]any
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, fmt.Errorf("failed to parse arguments: %w", err)
			}
		} else {
			args = make(map[string]any)
		}

		return handleToolCall(ctx, pool, query, types, args), nil
	}

	return tool, handler
}

// queryToolAnnotations builds the annotation hints for a query tool. Every
// query tool operates on a single closed database, so the world is closed. The
// read-only/destructive hints come from the query's EXPLAIN-based
// classification; when classification failed they are left at their
// conservative defaults (not read-only, destructive).
func queryToolAnnotations(query Query) *mcp.ToolAnnotations {
	annotations := &mcp.ToolAnnotations{
		OpenWorldHint: new(false),
	}

	if query.Classification != nil {
		annotations.ReadOnlyHint = query.Classification.ReadOnly
		if !query.Classification.ReadOnly {
			annotations.DestructiveHint = new(query.Classification.Destructive)
		}
	}

	return annotations
}

func buildInputSchema(query Query, meta *QueryMetadata) *jsonschema.Schema {
	properties := make(map[string]*jsonschema.Schema)
	var required []string

	for _, param := range query.Params {
		paramName := param.Column.Name
		if paramName == "" {
			paramName = fmt.Sprintf("param_%d", param.Number)
		}

		properties[paramName] = mapTypeToJSONSchema(param.Column, meta)

		// If not null, it's required
		if param.Column.NotNull {
			required = append(required, paramName)
		}
	}

	return &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func buildOutputSchema(query Query, meta *QueryMetadata) *jsonschema.Schema {
	// Handle :exec queries - they only return rows_affected
	if query.Cmd == ":exec" {
		return &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"rows_affected": {
					Type:        "integer",
					Description: "Number of rows affected by the operation",
				},
			},
			Required: []string{"rows_affected"},
		}
	}

	// Build properties from columns
	properties := make(map[string]*jsonschema.Schema)
	var required []string

	for _, col := range query.Columns {
		if col.Name == "" {
			// Skip unnamed columns
			continue
		}

		colSchema := mapTypeToJSONSchema(col, meta)

		// A column comment overrides any generic type description
		if col.Comment != "" {
			colSchema.Description = col.Comment
		}

		properties[col.Name] = colSchema

		// If not null, it's required
		if col.NotNull {
			required = append(required, col.Name)
		}
	}

	itemSchema := &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}

	// For :one queries, return the object schema directly
	if query.Cmd == ":one" {
		return itemSchema
	}

	// For :many queries, wrap the array in an object with a 'results' field
	// to satisfy MCP SDK's requirement that output schemas have type "object" at root
	if query.Cmd == ":many" {
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

	// Fallback for unknown query types
	return nil
}

// mapTypeToJSONSchema builds the JSON Schema for a column: the base type with
// any format, bounds, enum values, and length constraints, wrapped for array
// dimensions and nullability.
func mapTypeToJSONSchema(col Column, meta *QueryMetadata) *jsonschema.Schema {
	schema := baseTypeSchema(col, meta)

	if col.IsArray {
		// PostgreSQL array elements can always be NULL, regardless of the
		// column's own nullability.
		schema = allowNull(schema)

		// Wrap one array level per dimension. Only the innermost elements can
		// be null; sub-arrays of a multidimensional array cannot.
		for range max(col.ArrayDims, 1) {
			schema = &jsonschema.Schema{
				Type:  "array",
				Items: schema,
			}
		}
	}

	if !col.NotNull {
		schema = allowNull(schema)
	}

	return schema
}

// baseTypeSchema maps a PostgreSQL type to the JSON Schema describing how a
// single non-null value of that type appears in tool input and output.
func baseTypeSchema(col Column, meta *QueryMetadata) *jsonschema.Schema {
	switch col.Type.Name {
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
	case "text", "varchar", "character varying", "char", "character", "bpchar", "name", "citext", "xml":
		schema := &jsonschema.Schema{Type: "string"}
		if col.Length > 0 {
			schema.MaxLength = new(col.Length)
		}
		return schema
	default:
		// Enum types appear under their own name; list their values when the
		// catalog knows them.
		if enum := meta.FindEnum(col.Type); enum != nil {
			vals := make([]any, len(enum.Vals))
			for i, v := range enum.Vals {
				vals[i] = v
			}
			return &jsonschema.Schema{
				Type: "string",
				Enum: vals,
			}
		}
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
func convertParamValue(col Column, val any) (any, error) {
	if col.Type.Name != "bytea" {
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

func handleToolCall(ctx context.Context, pool *pgxpool.Pool, query Query, types []reflect.Type, input map[string]any) *mcp.CallToolResult {
	// Extract parameters in order
	var args []any
	for _, param := range query.Params {
		paramName := param.Column.Name
		if paramName == "" {
			paramName = fmt.Sprintf("param_%d", param.Number)
		}

		val, ok := input[paramName]
		if !ok && param.Column.NotNull {
			return errorResult("missing required parameter: %s", paramName)
		}

		val, err := convertParamValue(param.Column, val)
		if err != nil {
			return errorResult("invalid value for parameter %s: %v", paramName, err)
		}

		args = append(args, val)
	}

	// Execute query based on cmd type
	switch query.Cmd {
	case ":exec":
		return executeExec(ctx, pool, query, args)
	case ":one":
		return executeOne(ctx, pool, query, types, args)
	case ":many":
		return executeMany(ctx, pool, query, types, args)
	default:
		return errorResult("unknown query cmd: %s", query.Cmd)
	}
}

func executeExec(ctx context.Context, pool *pgxpool.Pool, query Query, args []any) *mcp.CallToolResult {
	tag, err := pool.Exec(ctx, query.Text, args...)
	if err != nil {
		return errorResult("query execution failed: %v", err)
	}

	result := map[string]any{
		"rows_affected": tag.RowsAffected(),
	}

	return successResult(result)
}

func executeOne(ctx context.Context, pool *pgxpool.Pool, query Query, types []reflect.Type, args []any) *mcp.CallToolResult {
	rows, err := pool.Query(ctx, query.Text, append([]any{textResults}, args...)...)
	if err != nil {
		return errorResult("query execution failed: %v", err)
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

func executeMany(ctx context.Context, pool *pgxpool.Pool, query Query, types []reflect.Type, args []any) *mcp.CallToolResult {
	rows, err := pool.Query(ctx, query.Text, append([]any{textResults}, args...)...)
	if err != nil {
		return errorResult("query execution failed: %v", err)
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
