package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

		result := runCommand(t, []string{"mcp", "uninstall", "claude-code"}, nil)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		if !removeCalled {
			t.Fatal("expected remove command to be called")
		}
		assertOutput(t, result.stdout, "CLIENT       STATUS       DETAIL  \nClaude Code  uninstalled          \n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("unconfigured_cli_client_exits_one", func(t *testing.T) {
		withMCPClientCommandRunner(t, func(ctx context.Context, command string, args ...string) ([]byte, error) {
			assertMCPClientCommand(t, command, args, "claude mcp get ghost")
			return []byte(`No MCP server found with name: "ghost". No MCP servers are configured.`), executableNotFoundError(command)
		})

		result := runCommand(t, []string{"mcp", "uninstall", "claude-code"}, nil)
		assertExitCode(t, result.err, mcpExitNoneConfigured)
		assertOutput(t, result.stdout, "CLIENT       STATUS        DETAIL  \nClaude Code  unconfigured          \n")
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
