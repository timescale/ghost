package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/common"
)

// defaultRowLimit caps how many result rows the browser-backed tools
// (ghost_visualize, ghost_ui_state) return to the agent. This prevents a large
// result set (potentially millions of rows) from being dumped into the LLM's
// context. The full result is still computed and charted in the browser;
// callers can raise it via the limit parameter.
const defaultRowLimit = 50

// resolveDatabase fetches the database by ref (which may be a name or an id)
// and returns it. Callers that need the canonical id (e.g. the web UI, which
// selects the database by id and reflects it in the URL) read database.Id;
// callers that connect to the database can also run common.CheckReady on the
// returned value before proceeding.
func resolveDatabase(ctx context.Context, client api.ClientWithResponsesInterface, projectID, databaseRef string) (api.Database, error) {
	resp, err := client.GetDatabaseWithResponse(ctx, projectID, databaseRef)
	if err != nil {
		return api.Database{}, fmt.Errorf("failed to get database: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return api.Database{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault)
	}
	if resp.JSON200 == nil {
		return api.Database{}, errors.New("empty response from API")
	}
	return *resp.JSON200, nil
}

// structuredOutputContent serializes a tool's structured output to a JSON
// [mcp.TextContent] block. The MCP spec requires that a tool returning
// structured content also return functionally-equivalent unstructured content,
// so a client can rely on the text content alone:
// https://modelcontextprotocol.io/specification/2025-06-18/server/tools#structured-content
//
// The go-sdk does this automatically, but only when the handler leaves
// CallToolResult.Content unset (see ToolHandlerFor). Handlers that set Content
// themselves (e.g. to attach a human-readable summary and an image) opt out of
// that auto-population, so they must prepend this block to keep the structured
// payload (e.g. query rows) visible to the model. Returns an error if the
// output can't be marshaled.
func structuredOutputContent(output any) (*mcp.TextContent, error) {
	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshaling structured output: %w", err)
	}
	return &mcp.TextContent{Text: string(data)}, nil
}

// Input property helpers

func databaseRefInputProperties(schema *jsonschema.Schema) {
	schema.Properties["name_or_id"].Description = "Database name or identifier"
}

func waitInputProperties(schema *jsonschema.Schema) {
	schema.Properties["wait"].Description = "Wait for the database to be ready before returning. Set to true if your next steps require connecting to or querying the database."
	schema.Properties["wait"].Default = json.RawMessage("false")
}

func createNameInputProperties(schema *jsonschema.Schema) {
	schema.Properties["name"].Description = "Database name (auto-generated if not provided)"
}

func forkNameInputProperties(schema *jsonschema.Schema) {
	schema.Properties["name"].Description = "Name for the forked database (defaults to '{source-name}-fork')"
}

func sizeInputProperties(schema *jsonschema.Schema) {
	schema.Properties["size"].Description = "Database size — controls the CPU and memory allocated to the database"
	schema.Properties["size"].Enum = []any{"1x", "2x", "4x", "8x"}
	schema.Properties["size"].Default = json.RawMessage(`"1x"`)
}

func shareTokenInputProperties(schema *jsonschema.Schema) {
	schema.Properties["share_token"].Description = "Share token from a database share. When provided, creates the new database from the shared snapshot."
}

// Output property helpers

func databaseIDOutputProperties(schema *jsonschema.Schema) {
	schema.Properties["id"].Description = "Database identifier"
}

func databaseNameOutputProperties(schema *jsonschema.Schema) {
	schema.Properties["name"].Description = "Database name"
}

func connectionStringOutputProperties(schema *jsonschema.Schema) {
	schema.Properties["connection_string"].Description = "PostgreSQL connection string"
}

func successOutputProperties(schema *jsonschema.Schema) {
	schema.Properties["success"].Description = "Whether the operation succeeded"
}

func warningsOutputProperties(schema *jsonschema.Schema) {
	schema.Properties["warnings"].Description = "Warnings encountered during the operation"
}

func sizeOutputProperties(schema *jsonschema.Schema) {
	schema.Properties["size"].Description = "Database size — represents the CPU and memory allocated to the database"
}
