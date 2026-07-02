package query

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
	"google.golang.org/protobuf/proto"
)

// RunPlugin implements the sqlc process plugin interface
func RunPlugin() error {
	// Read GenerateRequest from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}

	// Parse the request
	var req plugin.GenerateRequest
	if err := proto.Unmarshal(input, &req); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	// Extract query metadata
	queries := extractQueries(&req)

	// Parse plugin options for output filename
	filename := "queries.json"
	if len(req.PluginOptions) > 0 {
		var opts map[string]any
		if err := json.Unmarshal(req.PluginOptions, &opts); err == nil {
			if fn, ok := opts["filename"].(string); ok && fn != "" {
				filename = fn
			}
		}
	}

	// Build query data. The catalog's enum definitions (and the default
	// schema, needed to resolve unqualified type references against them)
	// ride along so the server can list enum values in the tools' JSON
	// Schemas.
	queryData := map[string]any{
		"queries":        queries,
		"enums":          extractEnums(&req),
		"default_schema": req.Catalog.GetDefaultSchema(),
	}

	jsonData, err := json.MarshalIndent(queryData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal queries: %w", err)
	}

	// Create response with a single file
	resp := &plugin.GenerateResponse{
		Files: []*plugin.File{
			{
				Name:     filename,
				Contents: jsonData,
			},
		},
	}

	// Write response to stdout
	output, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	w := bufio.NewWriter(os.Stdout)
	if _, err := w.Write(output); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %w", err)
	}

	return nil
}

// extractEnums flattens the enum types from the request's catalog. In
// database-only analyzer mode (no pg_dump) the catalog carries no schema
// information, so the result may be empty even when the database has enums.
func extractEnums(req *plugin.GenerateRequest) []Enum {
	var enums []Enum

	for _, schema := range req.Catalog.GetSchemas() {
		for _, enum := range schema.GetEnums() {
			enums = append(enums, Enum{
				Schema: schema.Name,
				Name:   enum.Name,
				Vals:   enum.Vals,
			})
		}
	}

	return enums
}

func extractQueries(req *plugin.GenerateRequest) []map[string]any {
	var queries []map[string]any

	for _, query := range req.Queries {
		q := map[string]any{
			"text":     query.Text,
			"name":     query.Name,
			"cmd":      query.Cmd,
			"filename": query.Filename,
			"comments": query.Comments,
		}

		// Extract columns
		var columns []map[string]any
		for _, col := range query.Columns {
			columns = append(columns, columnToMap(col))
		}
		q["columns"] = columns

		// Extract params
		var params []map[string]any
		for _, param := range query.Params {
			p := map[string]any{
				"number": param.Number,
			}
			if param.Column != nil {
				p["column"] = columnToMap(param.Column)
			}
			params = append(params, p)
		}
		q["params"] = params

		queries = append(queries, q)
	}

	return queries
}

func columnToMap(col *plugin.Column) map[string]any {
	if col == nil {
		return nil
	}

	m := map[string]any{
		"name":           col.Name,
		"not_null":       col.NotNull,
		"is_array":       col.IsArray,
		"comment":        col.Comment,
		"length":         col.Length,
		"is_named_param": col.IsNamedParam,
		"is_func_call":   col.IsFuncCall,
		"scope":          col.Scope,
		"table_alias":    col.TableAlias,
		"is_sqlc_slice":  col.IsSqlcSlice,
		"original_name":  col.OriginalName,
		"unsigned":       col.Unsigned,
		"array_dims":     col.ArrayDims,
		"embed_table":    nil,
	}

	if col.Table != nil {
		m["table"] = map[string]any{
			"catalog": col.Table.Catalog,
			"schema":  col.Table.Schema,
			"name":    col.Table.Name,
		}
	} else {
		m["table"] = map[string]any{
			"catalog": "",
			"schema":  "",
			"name":    "",
		}
	}

	if col.Type != nil {
		m["type"] = map[string]any{
			"catalog": col.Type.Catalog,
			"schema":  col.Type.Schema,
			"name":    col.Type.Name,
		}
	} else {
		m["type"] = map[string]any{
			"catalog": "",
			"schema":  "",
			"name":    "",
		}
	}

	return m
}
