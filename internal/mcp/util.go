package mcp

import (
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
)

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
