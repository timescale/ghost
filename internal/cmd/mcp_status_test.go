package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/timescale/ghost/internal/common"
)

func TestMCPStatusCmd(t *testing.T) {
	notFoundRunner := func(ctx context.Context, command string, args ...string) ([]byte, error) {
		return nil, executableNotFoundError(command)
	}

	cursorConfiguredFile := `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`

	cursorConfiguredOptBinFile := `{
  "mcpServers": {
    "ghost": {
      "command": "/opt/bin/ghost",
      "args": ["mcp", "start"]
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

	// Expected JSON output when every supported client reports
	// "not configured". Used by both the no-args and explicit-`all` tests
	// below to avoid duplicating the rendered table.
	allUnconfiguredJSON := "[\n" +
		"  {\n" +
		`    "client": "claude-code",` + "\n" +
		`    "status": "not configured"` + "\n" +
		"  },\n" +
		"  {\n" +
		`    "client": "cursor",` + "\n" +
		`    "status": "not configured"` + "\n" +
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
		"]\n"

	tests := []mcpCmdTest{
		{
			name: "configured cli client",
			args: []string{"mcp", "status", "claude-code"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertMCPClientCommand(t, command, args, "claude mcp get ghost")
				return []byte(`ghost:
  Scope: User config (available in all your projects)
  Status: ✗ Failed to connect
  Type: stdio
  Command: /Users/test/bin/ghost
  Args: mcp start
  Environment:
`), nil
			},
			wantStdout: "CLIENT       STATUS      \n" +
				"Claude Code  configured  \n",
		},
		{
			name: "unconfigured cli client exits two",
			args: []string{"mcp", "status", "codex"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertMCPClientCommand(t, command, args, "codex mcp list --json")
				return []byte(`[]`), nil
			},
			wantExitCode: mcpExitNoneConfigured,
			wantStdout: "CLIENT  STATUS          \n" +
				"Codex   not configured  \n",
		},
		{
			name: "executable not found is unconfigured",
			args: []string{"mcp", "status", "claude-code"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertMCPClientCommand(t, command, args, "claude mcp get ghost")
				return nil, executableNotFoundError(command)
			},
			wantExitCode: mcpExitNoneConfigured,
			wantStdout: "CLIENT       STATUS          \n" +
				"Claude Code  not configured  \n",
		},
		{
			name: "detection error exits one",
			args: []string{"mcp", "status", "codex"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertMCPClientCommand(t, command, args, "codex mcp list --json")
				return []byte(`not json`), nil
			},
			wantExitCode: common.ExitGeneralError,
			wantStdout: "CLIENT  STATUS  DETAIL                                                                                        \n" +
				"Codex   error   failed to parse codex mcp list output: invalid character 'o' in literal null (expecting 'u')  \n",
		},
		{
			name: "detection error with no output falls back to err message",
			args: []string{"mcp", "status", "claude-code"},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertMCPClientCommand(t, command, args, "claude mcp get ghost")
				return nil, errors.New("signal: killed")
			},
			wantExitCode: common.ExitGeneralError,
			wantStdout: "CLIENT       STATUS  DETAIL          \n" +
				"Claude Code  error   signal: killed  \n",
		},
		{
			name: "unexpected command in json file is unconfigured with detail",
			args: []string{"mcp", "status", "cursor"},
			files: map[string]string{
				".cursor/mcp.json": cursorUnexpectedCommandFile,
			},
			wantExitCode: mcpExitNoneConfigured,
			wantStdout: "CLIENT  STATUS          DETAIL                              \n" +
				"Cursor  not configured  ghost entry has unexpected command  \n",
		},
		{
			name: "configured json file client json output",
			args: []string{"mcp", "status", "cursor", "--json"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredOptBinFile,
			},
			wantStdout: "[\n" +
				"  {\n" +
				`    "client": "cursor",` + "\n" +
				`    "status": "configured"` + "\n" +
				"  }\n" +
				"]\n",
		},
		{
			name: "configured json file client yaml output",
			args: []string{"mcp", "status", "cursor", "--yaml"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredOptBinFile,
			},
			wantStdout: "- client: cursor\n  status: configured\n",
		},
		{
			name: "all clients mixed some configured exits zero",
			args: []string{"mcp", "status"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredFile,
			},
			runner: notFoundRunner,
			wantStdout: "CLIENT              STATUS          \n" +
				"Claude Code         not configured  \n" +
				"Cursor              configured      \n" +
				"Windsurf            not configured  \n" +
				"Codex               not configured  \n" +
				"Gemini CLI          not configured  \n" +
				"VS Code             not configured  \n" +
				"Google Antigravity  not configured  \n" +
				"Kiro CLI            not configured  \n",
		},
		{
			name:         "all clients no args all unconfigured exits two",
			args:         []string{"mcp", "status", "--json"},
			runner:       notFoundRunner,
			wantExitCode: mcpExitNoneConfigured,
			wantStdout:   allUnconfiguredJSON,
		},
		{
			name:         "explicit all target all unconfigured exits two",
			args:         []string{"mcp", "status", "all", "--json"},
			runner:       notFoundRunner,
			wantExitCode: mcpExitNoneConfigured,
			wantStdout:   allUnconfiguredJSON,
		},
		{
			name: "mixed configured and error exits one",
			args: []string{"mcp", "status", "--json"},
			files: map[string]string{
				".cursor/mcp.json": cursorConfiguredFile,
			},
			runner: func(ctx context.Context, command string, args ...string) ([]byte, error) {
				if command == "codex" {
					return []byte(`not json`), nil
				}
				return nil, executableNotFoundError(command)
			},
			// Configured (cursor) + error (codex) → detection error (1), per
			// mcpStatusExitCode.
			wantExitCode: common.ExitGeneralError,
			wantStdout: "[\n" +
				"  {\n" +
				`    "client": "claude-code",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "cursor",` + "\n" +
				`    "status": "configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "windsurf",` + "\n" +
				`    "status": "not configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "codex",` + "\n" +
				`    "status": "error",` + "\n" +
				`    "detail": "failed to parse codex mcp list output: invalid character 'o' in literal null (expecting 'u')"` + "\n" +
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
		},
	}

	runMCPCmdTests(t, tests)
}
