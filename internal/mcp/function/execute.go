package function

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func handleToolCall(ctx context.Context, pool *pgxpool.Pool, tool tool, types []reflect.Type, input map[string]any) *mcp.CallToolResult {
	sql, args, err := buildCall(tool, input)
	if err != nil {
		return errorResult("%v", err)
	}

	switch tool.Mode {
	case modeExec:
		return executeExec(ctx, pool, sql, args)
	case modeOne:
		return executeOne(ctx, pool, sql, types, args)
	case modeMany:
		return executeMany(ctx, pool, sql, types, args)
	default:
		return errorResult("unknown tool mode: %s", tool.Mode)
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
func buildCall(tool tool, input map[string]any) (string, []any, error) {
	type callArg struct {
		param param
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
		if len(omitted) > 0 && !tool.NamedArgs {
			return "", nil, fmt.Errorf("parameter %s requires %s to also be provided (the function's arguments are unnamed, so defaults can only be omitted from the end)", param.Name, strings.Join(omitted, ", "))
		}
		provided = append(provided, callArg{param: param, value: val})
	}

	parts := make([]string, len(provided))
	values := make([]any, len(provided))
	useNamed := tool.NamedArgs && len(omitted) > 0
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
	if tool.Mode == modeExec {
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

	// Not nil even for zero rows: the output schema declares "results" as a
	// required array, and a nil slice would marshal to null instead of [].
	results := make([]map[string]any, 0)
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

// convertParamValue converts a JSON argument into the value pgx should send
// for the parameter's PostgreSQL type. Most JSON values pass through as-is,
// but bytea parameters arrive as base64 strings (as their schema advertises)
// and must be decoded; sending the string unchanged would store the base64
// text itself.
func convertParamValue(typ typeInfo, val any) (any, error) {
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
