package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/common"
)

func TestMCPUninstallCmd(t *testing.T) {
	falseVal := false
	trueVal := true

	cursorConfiguredFile := `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`

	cursorConfiguredWithOtherFile := `{
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
}`

	cursorUnexpectedCommandFile := `{
  "mcpServers": {
    "ghost": {
      "command": "/some/other/binary",
      "args": ["mcp", "start"]
    }
  }
}`

	// configuredCLIRemoveCalled is set true by the "configured cli client"
	// test's runner when the production code invokes
	// `claude mcp remove -s user ghost`. Only that test reads/writes it; other
	// test cases use their own runners and never touch it.
	var configuredCLIRemoveCalled bool

	tests := []mcpCmdTest{
		{
			name: "configured cli client",
			args: []string{"mcp", "uninstall", "claude-code", "--no-backup"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				switch got := commandWithArgs(command, args); got {
				case "claude mcp get ghost":
					return []byte(`ghost:
  Command: ghost
  Args: mcp start
`), nil
				case "claude mcp remove -s user ghost":
					configuredCLIRemoveCalled = true
					return []byte("Removed MCP server ghost from user config\n"), nil
				default:
					t.Fatalf("unexpected command: %s", got)
					return nil, nil
				}
			},
			wantStdout: "CLIENT       STATUS       \n" +
				"Claude Code  uninstalled  \n",
			after: func(t *testing.T, _ string) {
				if !configuredCLIRemoveCalled {
					t.Fatal("expected remove command to be called")
				}
			},
		},
		{
			name: "configured cli client creates backup by default",
			args: []string{"mcp", "uninstall", "claude-code"},
			files: map[string]string{
				".claude.json": `{"mcpServers":{"ghost":{"command":"ghost","args":["mcp","start"]}}}`,
			},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
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
			},
			wantStdout: "CLIENT       STATUS       \n" +
				"Claude Code  uninstalled  \n",
			after: func(t *testing.T, homeDir string) {
				entries, err := os.ReadDir(homeDir)
				if err != nil {
					t.Fatalf("failed to read home dir: %v", err)
				}
				for _, e := range entries {
					if strings.HasPrefix(e.Name(), ".claude.json.backup.") {
						return
					}
				}
				names := make([]string, len(entries))
				for i, e := range entries {
					names[i] = e.Name()
				}
				t.Fatalf("expected a backup of .claude.json to be created, got entries: %v", names)
			},
		},
		{
			name: "unconfigured cli client exits two",
			args: []string{"mcp", "uninstall", "claude-code"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertMCPClientCommand(t, command, args, "claude mcp get ghost")
				return []byte(`No MCP server found with name: "ghost". No MCP servers are configured.`), executableNotFoundError(command)
			},
			wantExitCode: mcpExitNoneConfigured,
			wantStdout: "CLIENT       STATUS          \n" +
				"Claude Code  not configured  \n",
		},
		{
			name: "detection error propagates",
			args: []string{"mcp", "uninstall", "codex"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertMCPClientCommand(t, command, args, "codex mcp list --json")
				return []byte(`not json`), nil
			},
			wantExitCode: common.ExitGeneralError,
			wantStdout: "CLIENT  STATUS  DETAIL                                                                                        \n" +
				"Codex   error   failed to parse codex mcp list output: invalid character 'o' in literal null (expecting 'u')  \n",
		},
		{
			name: "cli uninstall with no output falls back to err message",
			args: []string{"mcp", "uninstall", "claude-code", "--no-backup"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
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
			},
			wantExitCode: common.ExitGeneralError,
			wantStdout: "CLIENT       STATUS  DETAIL          \n" +
				"Claude Code  error   signal: killed  \n",
		},
		{
			name: "configured json file client",
			args: []string{"mcp", "uninstall", "cursor", "--no-backup"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredWithOtherFile,
			},
			wantStdout: "CLIENT  STATUS       \n" +
				"Cursor  uninstalled  \n",
			after: func(t *testing.T, homeDir string) {
				content, err := os.ReadFile(filepath.Join(homeDir, ".cursor", "mcp.json"))
				if err != nil {
					t.Fatalf("failed to read cursor config: %v", err)
				}
				want := "{\n\t\"mcpServers\": {\n\t\t\"other\": {\n\t\t\t\"command\": \"other\",\n\t\t\t\"args\":    []\n\t\t}\n\t}\n}\n"
				assertOutput(t, string(content), want)
			},
		},
		{
			name: "unexpected command in json file is left alone",
			args: []string{"mcp", "uninstall", "cursor", "--no-backup"},
			files: map[string]string{
				".cursor/mcp.json": cursorUnexpectedCommandFile,
			},
			// Detection sees a not-configured (unexpected command) entry, so
			// we exit 2 without modifying the file.
			wantExitCode: mcpExitNoneConfigured,
			wantStdout: "CLIENT  STATUS          DETAIL                              \n" +
				"Cursor  not configured  ghost entry has unexpected command  \n",
			after: func(t *testing.T, homeDir string) {
				content, err := os.ReadFile(filepath.Join(homeDir, ".cursor", "mcp.json"))
				if err != nil {
					t.Fatalf("failed to read cursor config: %v", err)
				}
				if string(content) != cursorUnexpectedCommandFile {
					t.Fatalf("expected config to be unchanged, got:\n%s", string(content))
				}
			},
		},
		{
			name: "json output",
			args: []string{"mcp", "uninstall", "cursor", "--no-backup", "--json"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredFile,
			},
			wantStdout: "[\n" +
				"  {\n" +
				`    "client": "cursor",` + "\n" +
				`    "status": "uninstalled"` + "\n" +
				"  }\n" +
				"]\n",
		},
		{
			name: "yaml output",
			args: []string{"mcp", "uninstall", "cursor", "--no-backup", "--yaml"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredFile,
			},
			wantStdout: "- client: cursor\n  status: uninstalled\n",
		},
		{
			name: "all target uninstalls configured clients",
			args: []string{"mcp", "uninstall", "all", "--no-backup", "--json"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredFile,
			},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				return nil, executableNotFoundError(command)
			},
			wantStdout: "[\n" +
				"  {\n" +
				`    "client": "claude-code",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "cursor",` + "\n" +
				`    "status": "uninstalled"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "windsurf",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "codex",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "gemini",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "vscode",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "antigravity",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "kiro-cli",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  }\n" +
				"]\n",
			after: func(t *testing.T, homeDir string) {
				content, err := os.ReadFile(filepath.Join(homeDir, ".cursor", "mcp.json"))
				if err != nil {
					t.Fatalf("failed to read cursor config: %v", err)
				}
				if strings.Contains(string(content), "ghost") {
					t.Fatalf("expected ghost entry to be removed from cursor config, got:\n%s", string(content))
				}
			},
		},
		{
			name: "interactive selection via stub",
			args: []string{"mcp", "uninstall", "--no-backup"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredFile,
			},
			uninstallSelector: func(_ *cobra.Command) (string, error) {
				return "cursor", nil
			},
			isTerminal: &trueVal,
			wantStdout: "CLIENT  STATUS       \n" +
				"Cursor  uninstalled  \n",
		},
		{
			name:       "no client non terminal",
			args:       []string{"mcp", "uninstall"},
			isTerminal: &falseVal,
			wantErr:    "no client specified and stdin is not a terminal; pass the client name or 'all' as an argument",
		},
	}

	runMCPCmdTests(t, tests)
}
