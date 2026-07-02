package query

import (
	_ "embed"
	"fmt"
	"os"
	"text/template"
)

//go:embed template.yaml
var sqlcConfigTemplate string

// SqlcConfig contains parameters for generating sqlc.yaml.
//
// SchemaPath is optional: when set, sqlc parses the schema file and combines it
// with live-database analysis (hybrid mode). When empty, the config instead
// enables sqlc's database-only analyzer mode (analyzer.database: only), which
// resolves everything from the database connection alone.
type SqlcConfig struct {
	SchemaPath  string
	QueriesPath string
	OutPath     string
	PluginCmd   string
	DatabaseURL string
}

// GenerateSqlcConfig creates a sqlc.yaml configuration file
func GenerateSqlcConfig(sqlcConfigPath string, cfg SqlcConfig) error {
	// Parse template
	tmpl, err := template.New("sqlc").Parse(sqlcConfigTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Create config file
	f, err := os.Create(sqlcConfigPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	// Execute template
	if err := tmpl.Execute(f, cfg); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return nil
}
