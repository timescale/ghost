package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMCPUninstallCmd(t *testing.T) {
	t.Run("configured_cli_client", func(t *testing.T) {
		var removeCalled bool
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			switch got := commandWithArgs(command, args); got {
			case "claude mcp get ghost":
				return []byte(`ghost:
  Command: ghost
  Args: mcp start
`), nil
			case "claude mcp remove -s user ghost":
				removeCalled = true
				return []byte("Removed MCP server ghost from user config\n"), nil
			default:
				t.Fatalf("unexpected command: %s", got)
				return nil, nil
			}
		})

		result := runCommand(t, []string{"mcp", "uninstall", "claude-code", "--no-backup"}, nil)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		if !removeCalled {
			t.Fatal("expected remove command to be called")
		}
		assertOutput(t, result.stdout, "CLIENT       STATUS       DETAIL  \nClaude Code  uninstalled          \n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("configured_cli_client_creates_backup_by_default", func(t *testing.T) {
		homeDir := t.TempDir()
		claudeConfigPath := filepath.Join(homeDir, ".claude.json")
		writeTestFile(t, claudeConfigPath, `{"mcpServers":{"ghost":{"command":"ghost","args":["mcp","start"]}}}`)

		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			switch got := commandWithArgs(command, args); got {
			case "claude mcp get ghost":
				return []byte(`ghost:
  Command: ghost
  Args: mcp start
`), nil
			case "claude mcp remove -s user ghost":
				return []byte("Removed MCP server ghost from user config\n"), nil
			default:
				t.Fatalf("unexpected command: %s", got)
				return nil, nil
			}
		})

		result := runCommand(t, []string{"mcp", "uninstall", "claude-code"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}

		entries, err := os.ReadDir(homeDir)
		if err != nil {
			t.Fatalf("failed to read home dir: %v", err)
		}
		var backupFound bool
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".claude.json.backup.") {
				backupFound = true
				break
			}
		}
		if !backupFound {
			names := make([]string, len(entries))
			for i, e := range entries {
				names[i] = e.Name()
			}
			t.Fatalf("expected a backup of .claude.json to be created, got entries: %v", names)
		}
	})

	t.Run("unconfigured_cli_client_exits_two", func(t *testing.T) {
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			assertMCPClientCommand(t, command, args, "claude mcp get ghost")
			return []byte(`No MCP server found with name: "ghost". No MCP servers are configured.`), executableNotFoundError(command)
		})

		result := runCommand(t, []string{"mcp", "uninstall", "claude-code"}, nil)
		assertExitCode(t, result.err, mcpExitNotConfigured)
		assertOutput(t, result.stdout, "CLIENT       STATUS        DETAIL  \nClaude Code  unconfigured          \n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("detection_error_propagates", func(t *testing.T) {
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			assertMCPClientCommand(t, command, args, "codex mcp list --json")
			return []byte(`not json`), nil
		})

		result := runCommand(t, []string{"mcp", "uninstall", "codex"}, nil)
		assertExitCode(t, result.err, mcpExitNotConfigured)
		assertOutput(t, result.stdout, "CLIENT  STATUS  DETAIL                                                                                        \nCodex   error   failed to parse codex mcp list output: invalid character 'o' in literal null (expecting 'u')  \n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("cli_uninstall_with_no_output_falls_back_to_err_message", func(t *testing.T) {
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			switch got := commandWithArgs(command, args); got {
			case "claude mcp get ghost":
				return []byte(`ghost:
  Command: ghost
  Args: mcp start
`), nil
			case "claude mcp remove -s user ghost":
				return nil, errors.New("signal: killed")
			default:
				t.Fatalf("unexpected command: %s", got)
				return nil, nil
			}
		})

		result := runCommand(t, []string{"mcp", "uninstall", "claude-code", "--no-backup"}, nil)
		assertExitCode(t, result.err, mcpExitNotConfigured)
		assertOutput(t, result.stdout, "CLIENT       STATUS  DETAIL          \nClaude Code  error   signal: killed  \n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("configured_json_file_client", func(t *testing.T) {
		homeDir := t.TempDir()
		cursorConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		writeTestFile(t, cursorConfigPath, `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    },
    "other": {
      "command": "other",
      "args": []
    }
  }
}`)

		result := runCommand(t, []string{"mcp", "uninstall", "cursor", "--no-backup"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "CLIENT  STATUS       DETAIL  \nCursor  uninstalled          \n")
		assertOutput(t, result.stderr, "")

		content, err := os.ReadFile(cursorConfigPath)
		if err != nil {
			t.Fatalf("failed to read cursor config: %v", err)
		}
		assertOutput(t, string(content), "{\n\t\"mcpServers\": {\n\t\t\"other\": {\n\t\t\t\"command\": \"other\",\n\t\t\t\"args\":    []\n\t\t}\n\t}\n}\n")
	})

	t.Run("unexpected_command_in_json_file_is_left_alone", func(t *testing.T) {
		homeDir := t.TempDir()
		cursorConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		original := `{
  "mcpServers": {
    "ghost": {
      "command": "/some/other/binary",
      "args": ["mcp", "start"]
    }
  }
}`
		writeTestFile(t, cursorConfigPath, original)

		result := runCommand(t, []string{"mcp", "uninstall", "cursor", "--no-backup"}, nil, withEnv("HOME", homeDir))
		// Detection sees an unconfigured (unexpected command) entry, so we exit 2 without modifying the file.
		assertExitCode(t, result.err, mcpExitNotConfigured)
		assertOutput(t, result.stdout, "CLIENT  STATUS        DETAIL                              \nCursor  unconfigured  ghost entry has unexpected command  \n")
		assertOutput(t, result.stderr, "")

		content, err := os.ReadFile(cursorConfigPath)
		if err != nil {
			t.Fatalf("failed to read cursor config: %v", err)
		}
		// File must be byte-for-byte unchanged.
		if string(content) != original {
			t.Fatalf("expected config to be unchanged, got:\n%s", string(content))
		}
	})

	t.Run("json_output", func(t *testing.T) {
		homeDir := t.TempDir()
		cursorConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		writeTestFile(t, cursorConfigPath, `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`)

		result := runCommand(t, []string{"mcp", "uninstall", "cursor", "--no-backup", "--json"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `[
  {
    "client": "Cursor",
    "status": "uninstalled"
  }
]
`)
		assertOutput(t, result.stderr, "")
	})

	t.Run("yaml_output", func(t *testing.T) {
		homeDir := t.TempDir()
		cursorConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		writeTestFile(t, cursorConfigPath, `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`)

		result := runCommand(t, []string{"mcp", "uninstall", "cursor", "--no-backup", "--yaml"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `- client: Cursor
  status: uninstalled
`)
		assertOutput(t, result.stderr, "")
	})

	t.Run("all_target_uninstalls_configured_clients", func(t *testing.T) {
		homeDir := t.TempDir()
		// Configure cursor only; everything else looks absent.
		cursorConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		writeTestFile(t, cursorConfigPath, `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`)
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			return nil, executableNotFoundError(command)
		})

		result := runCommand(t, []string{"mcp", "uninstall", "all", "--no-backup", "--json"}, nil, withEnv("HOME", homeDir))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		// Cursor should be "uninstalled"; all other clients should be "unconfigured".
		if !strings.Contains(result.stdout, `"client": "Cursor"`) || !strings.Contains(result.stdout, `"status": "uninstalled"`) {
			t.Fatalf("expected Cursor to be uninstalled, got:\n%s", result.stdout)
		}
		// Confirm cursor's ghost entry was removed.
		content, err := os.ReadFile(cursorConfigPath)
		if err != nil {
			t.Fatalf("failed to read cursor config: %v", err)
		}
		if strings.Contains(string(content), "ghost") {
			t.Fatalf("expected ghost entry to be removed from cursor config, got:\n%s", string(content))
		}
	})

	t.Run("interactive_selection_via_stub", func(t *testing.T) {
		// Override the interactive selector so we don't need a TTY.
		originalSelector := uninstallTargetSelector
		uninstallTargetSelector = func(_ *cobra.Command) (string, error) {
			return "cursor", nil
		}
		t.Cleanup(func() { uninstallTargetSelector = originalSelector })

		homeDir := t.TempDir()
		cursorConfigPath := filepath.Join(homeDir, ".cursor", "mcp.json")
		writeTestFile(t, cursorConfigPath, `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`)

		result := runCommand(t, []string{"mcp", "uninstall", "--no-backup"}, nil,
			withEnv("HOME", homeDir),
			withStdin(""),
			withIsTerminal(true),
		)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "CLIENT  STATUS       DETAIL  \nCursor  uninstalled          \n")
	})

	t.Run("no_client_non_terminal", func(t *testing.T) {
		result := runCommand(t, []string{"mcp", "uninstall"}, nil, withStdin(""), withIsTerminal(false))
		assertOutput(t, result.err.Error(), "no client specified and stdin is not a terminal; pass the client name or 'all' as an argument")
		assertOutput(t, result.stdout, "")
		assertOutput(t, result.stderr, "Error: no client specified and stdin is not a terminal; pass the client name or 'all' as an argument\n")
	})
}

func commandWithArgs(command string, args []string) string {
	return strings.Join(append([]string{command}, args...), " ")
}
