package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withGhostExecutablePath overrides the ghost executable path resolver so the
// install command can produce deterministic output without depending on the
// real binary location.
func withGhostExecutablePath(t *testing.T, path string) {
	t.Helper()
	original := ghostExecutablePathFunc
	ghostExecutablePathFunc = func() (string, error) { return path, nil }
	t.Cleanup(func() { ghostExecutablePathFunc = original })
}

func TestMCPInstallCmd(t *testing.T) {
	t.Run("single_client_text_output", func(t *testing.T) {
		homeDir := t.TempDir()
		withGhostExecutablePath(t, "/opt/bin/ghost")

		result := runCommand(t, []string{"mcp", "install", "cursor", "--no-backup"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		expectedConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		expectedStdout := "Successfully installed Ghost MCP server configuration for cursor\n" +
			"Configuration file: " + expectedConfigPath + "\n\n" +
			"Next steps:\n" +
			"   1. Restart cursor to load the new configuration\n" +
			"   2. The Ghost MCP server will be available as 'ghost'\n"
		assertOutput(t, result.stdout, expectedStdout)
		assertOutput(t, result.stderr, "")

		content, err := os.ReadFile(expectedConfigPath)
		if err != nil {
			t.Fatalf("failed to read cursor config: %v", err)
		}
		if !strings.Contains(string(content), `"ghost"`) {
			t.Fatalf("expected ghost entry in cursor config, got: %s", string(content))
		}
	})

	t.Run("single_client_json_output", func(t *testing.T) {
		homeDir := t.TempDir()
		withGhostExecutablePath(t, "/opt/bin/ghost")

		result := runCommand(t, []string{"mcp", "install", "cursor", "--no-backup", "--json"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		expectedConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		expectedJSON := `[
  {
    "client": "Cursor",
    "status": "installed",
    "detail": "` + expectedConfigPath + `"
  }
]
`
		assertOutput(t, result.stdout, expectedJSON)
		assertOutput(t, result.stderr, "")
	})

	t.Run("single_client_yaml_output", func(t *testing.T) {
		homeDir := t.TempDir()
		withGhostExecutablePath(t, "/opt/bin/ghost")

		result := runCommand(t, []string{"mcp", "install", "cursor", "--no-backup", "--yaml"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		expectedConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		expectedYAML := "- client: Cursor\n  detail: " + expectedConfigPath + "\n  status: installed\n"
		assertOutput(t, result.stdout, expectedYAML)
		assertOutput(t, result.stderr, "")
	})

	t.Run("json_and_yaml_are_mutually_exclusive", func(t *testing.T) {
		result := runCommand(t, []string{"mcp", "install", "cursor", "--json", "--yaml"}, nil)
		if result.err == nil {
			t.Fatal("expected error when both --json and --yaml are provided")
		}
		if !strings.Contains(result.err.Error(), "[json yaml]") {
			t.Fatalf("expected mutually exclusive error mentioning [json yaml], got: %v", result.err)
		}
	})

	t.Run("no_client_non_terminal", func(t *testing.T) {
		result := runCommand(t, []string{"mcp", "install"}, nil, withStdin(""), withIsTerminal(false))
		assertOutput(t, result.err.Error(), "no client specified and stdin is not a terminal; pass the client name or 'all' as an argument")
	})

	t.Run("all_with_config_path_errors", func(t *testing.T) {
		result := runCommand(t, []string{"mcp", "install", "all", "--config-path", "/tmp/foo.json"}, nil)
		if result.err == nil {
			t.Fatal("expected error when --config-path is used with all")
		}
		assertOutput(t, result.err.Error(), "--config-path cannot be used with 'all'")
	})

	t.Run("all_target_skips_already_configured_and_installs_json_clients", func(t *testing.T) {
		homeDir := t.TempDir()
		withGhostExecutablePath(t, "/opt/bin/ghost")

		// Stub all CLI-based detection to report the ghost server is already
		// configured. installGhostMCPForAllClients then skips the install path
		// for those clients, which keeps the test from shelling out to real
		// `claude` / `codex` / `gemini` / `kiro-cli` binaries.
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			cmdLine := commandWithArgs(command, args)
			switch {
			case strings.HasPrefix(cmdLine, "claude mcp get"):
				return []byte("ghost:\n  Command: ghost\n  Args: mcp start\n"), nil
			case strings.HasPrefix(cmdLine, "codex mcp list"):
				return []byte(`[{"name":"ghost","transport":{"command":"ghost","args":["mcp","start"]}}]`), nil
			case strings.HasPrefix(cmdLine, "gemini mcp list"):
				return []byte("  ghost: ghost mcp start (stdio) - Ready"), nil
			case strings.HasPrefix(cmdLine, "kiro-cli mcp status"):
				return []byte("Command: ghost\n"), nil
			default:
				return nil, errors.New("unexpected command: " + cmdLine)
			}
		})

		// Pre-create config files for clients that should be detected as already
		// configured. Kiro's CLI status output does not include args, so the
		// detector verifies the file too. VS Code installs through the `code` CLI,
		// which is not available in CI, so keep it on the already-configured path.
		kiroConfigPath := filepath.Join(homeDir, ".kiro", "settings", "mcp.json")
		writeTestFile(t, kiroConfigPath, `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`)
		vscodeConfigPath := filepath.Join(homeDir, ".config", "Code", "User", "mcp.json")
		writeTestFile(t, vscodeConfigPath, `{
  "servers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`)

		result := runCommand(t, []string{"mcp", "install", "all", "--no-backup", "--json"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}

		// Cursor / Windsurf / Antigravity are JSON-config clients with no CLI
		// detection, so detection returns "unconfigured" and install proceeds.
		cursorConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		content, err := os.ReadFile(cursorConfigPath)
		if err != nil {
			t.Fatalf("failed to read cursor config: %v", err)
		}
		if !strings.Contains(string(content), `"ghost"`) {
			t.Fatalf("expected ghost entry in cursor config, got: %s", string(content))
		}

		stdout := result.stdout
		if !strings.Contains(stdout, `"client": "Cursor"`) || !strings.Contains(stdout, `"status": "installed"`) {
			t.Fatalf("expected installed row for Cursor, got: %s", stdout)
		}
		// Claude Code / Codex / Gemini / Kiro CLI all detected as configured.
		if !strings.Contains(stdout, `"client": "Claude Code"`) || !strings.Contains(stdout, `"status": "already configured"`) {
			t.Fatalf("expected already-configured row for Claude Code, got: %s", stdout)
		}
	})
}
