package query

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/sqlc-dev/sqlc/pkg/cli"
)

// SqlcConfigEnvVar names the environment variable that, when set, puts the
// binary into "sqlc runner" mode: it runs sqlc against the referenced config
// file and exits. See RunSqlcDirect and RunSqlc.
const SqlcConfigEnvVar = "GHOST_SQLC_RUNNER_CONFIG"

// RunSqlc invokes sqlc to process queries by spawning the current executable in
// "sqlc runner" mode (see SqlcConfigEnvVar) as a subprocess.
//
// sqlc is run out-of-process for two reasons:
//  1. sqlc's `generate` command calls os.Exit(1) on failure (e.g. invalid SQL),
//     which would otherwise terminate the long-running MCP server. Isolating it
//     in a subprocess turns that into an ordinary non-zero exit code.
//  2. sqlc writes to stdout/stderr, which would corrupt the MCP server's stdio
//     transport. A subprocess has its own streams, which we capture here.
//
// On failure the captured output (which includes sqlc's diagnostics) is
// included in the returned error so callers can surface and act on it.
func RunSqlc(ctx context.Context, sqlcConfigPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.CommandContext(ctx, exePath)
	cmd.Env = append(os.Environ(), SqlcConfigEnvVar+"="+sqlcConfigPath)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sqlc failed: %w\n%s", err, output.String())
	}

	return nil
}

// RunSqlcDirect runs sqlc in-process against the given config file and returns
// its exit code. It is invoked by the "sqlc runner" subprocess spawned by
// RunSqlc.
//
// Note that sqlc may call os.Exit itself, so this function is not guaranteed to
// return; callers should treat it as terminal.
func RunSqlcDirect(sqlcConfigPath string) int {
	// When no schema file is available, the generated config sets
	// `analyzer.database: only`, which makes sqlc resolve all schema and type
	// information from the live database connection instead of schema files.
	// That mode is gated behind sqlc's analyzerv2 experiment, so enable the
	// experiment unconditionally here; it has no effect unless the config asks
	// for database-only mode.
	os.Setenv("SQLCEXPERIMENT", "analyzerv2")

	// sqlc cli.Run takes args (excluding program name) and returns an exit code.
	// We handle the protobuf conflict with a linker flag (see the repository's
	// build configuration).
	return cli.Run([]string{"generate", "-f", sqlcConfigPath})
}
