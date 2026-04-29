package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPStatusCmd(t *testing.T) {
	t.Run("configured_cli_client", func(t *testing.T) {
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			assertMCPClientCommand(t, command, args, "claude mcp get ghost")
			return []byte(`ghost:
  Scope: User config (available in all your projects)
  Status: ✗ Failed to connect
  Type: stdio
  Command: /Users/test/bin/ghost
  Args: mcp start
  Environment:
`), nil
		})

		result := runCommand(t, []string{"mcp", "status", "claude-code"}, nil)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "CLIENT       STATUS      DETAIL  \nClaude Code  configured          \n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("unconfigured_cli_client_exits_one", func(t *testing.T) {
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			assertMCPClientCommand(t, command, args, "codex mcp list --json")
			return []byte(`[]`), nil
		})

		result := runCommand(t, []string{"mcp", "status", "codex"}, nil)
		assertExitCode(t, result.err, mcpExitNoneConfigured)
		assertOutput(t, result.stdout, "CLIENT  STATUS        DETAIL  \nCodex   unconfigured          \n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("detection_error_exits_two", func(t *testing.T) {
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			assertMCPClientCommand(t, command, args, "codex mcp list --json")
			return []byte(`not json`), nil
		})

		result := runCommand(t, []string{"mcp", "status", "codex"}, nil)
		assertExitCode(t, result.err, mcpExitDetectionError)
		assertOutput(t, result.stdout, "CLIENT  STATUS  DETAIL                                                                                        \nCodex   error   failed to parse codex mcp list output: invalid character 'o' in literal null (expecting 'u')  \n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("configured_json_file_client", func(t *testing.T) {
		homeDir := t.TempDir()
		cursorConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		writeTestFile(t, cursorConfigPath, `{
  "mcpServers": {
    "ghost": {
      "command": "/opt/bin/ghost",
      "args": ["mcp", "start"]
    }
  }
}`)

		result := runCommand(t, []string{"mcp", "status", "cursor", "--json"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `[
  {
    "client": "Cursor",
    "client_name": "cursor",
    "status": "configured"
  }
]
`)
		assertOutput(t, result.stderr, "")
	})
}

func withMCPClientCommandRunner(t *testing.T, runner mcpClientCommandRunner) {
	t.Helper()
	originalRunner := runMCPClientCommand
	runMCPClientCommand = runner
	t.Cleanup(func() { runMCPClientCommand = originalRunner })
}

func assertMCPClientCommand(t *testing.T, command string, args []string, want string) {
	t.Helper()
	got := strings.Join(append([]string{command}, args...), " ")
	if got != want {
		t.Fatalf("command mismatch: got %q, want %q", got, want)
	}
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected exit code %d, got nil error", want)
	}
	exitErr, ok := err.(interface{ ExitCode() int })
	if !ok {
		t.Fatalf("expected exit code %d, got non-exit error: %v", want, err)
	}
	if got := exitErr.ExitCode(); got != want {
		t.Fatalf("exit code mismatch: got %d, want %d", got, want)
	}
	assertOutput(t, err.Error(), "")
}

func executableNotFoundError(command string) error {
	return &exec.Error{Name: command, Err: exec.ErrNotFound}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}
