package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPInstallCmd(t *testing.T) {
	falseVal := false

	// allConfiguredRunner stubs every CLI-based detection call to report that
	// the ghost server is already configured. Combined with pre-created VS
	// Code and Kiro CLI config files, this lets the "all" test exercise
	// installGhostMCPForAllClients without shelling out to real client
	// binaries (claude / codex / gemini / kiro-cli).
	allConfiguredRunner := func(ctx context.Context, command string, args ...string) ([]byte, error) {
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
	}

	// Pre-existing config files that mark VS Code and Kiro CLI as already
	// configured for the "all" test. Kiro's CLI status output omits args, so
	// the detector also verifies the file. VS Code installs through the
	// `code` CLI, which is not available in CI, so we keep it on the
	// already-configured path.
	allConfiguredFiles := map[string]string{
		".kiro/settings/mcp.json": `{
  "mcpServers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`,
		".config/Code/User/mcp.json": `{
  "servers": {
    "ghost": {
      "command": "ghost",
      "args": ["mcp", "start"]
    }
  }
}`,
	}

	// Asserts that the cursor config now contains a ghost entry. Used by
	// tests that install for cursor (directly or as part of "all").
	assertCursorHasGhost := func(t *testing.T, homeDir string) {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(homeDir, ".cursor", "mcp.json"))
		if err != nil {
			t.Fatalf("failed to read cursor config: %v", err)
		}
		if !strings.Contains(string(content), `"ghost"`) {
			t.Fatalf("expected ghost entry in cursor config, got: %s", string(content))
		}
	}

	tests := []mcpCmdTest{
		{
			name:      "single client text output",
			args:      []string{"mcp", "install", "cursor", "--no-backup"},
			ghostPath: "/opt/bin/ghost",
			wantStdoutFunc: func(homeDir string) string {
				configPath := homeDir + "/.cursor/mcp.json"
				detailPad := strings.Repeat(" ", len(configPath)-len("DETAIL"))
				return "CLIENT  STATUS     DETAIL" + detailPad + "  \n" +
					"Cursor  installed  " + configPath + "  \n" +
					"\n" +
					"Next steps:\n" +
					"   1. Restart Cursor to load the new configuration\n" +
					"   2. The Ghost MCP server will be available as 'ghost'\n"
			},
			after: assertCursorHasGhost,
		},
		{
			name:      "single client json output",
			args:      []string{"mcp", "install", "cursor", "--no-backup", "--json"},
			ghostPath: "/opt/bin/ghost",
			wantStdout: "[\n" +
				"  {\n" +
				`    "client": "cursor",` + "\n" +
				`    "status": "installed",` + "\n" +
				`    "detail": "{{HOME}}/.cursor/mcp.json"` + "\n" +
				"  }\n" +
				"]\n",
			after: assertCursorHasGhost,
		},
		{
			name:       "single client yaml output",
			args:       []string{"mcp", "install", "cursor", "--no-backup", "--yaml"},
			ghostPath:  "/opt/bin/ghost",
			wantStdout: "- client: cursor\n  detail: {{HOME}}/.cursor/mcp.json\n  status: installed\n",
			after:      assertCursorHasGhost,
		},
		{
			name:    "json and yaml are mutually exclusive",
			args:    []string{"mcp", "install", "cursor", "--json", "--yaml"},
			wantErr: "if any flags in the group [json yaml] are set none of the others can be; [json yaml] were all set",
		},
		{
			name:       "no client non terminal",
			args:       []string{"mcp", "install"},
			isTerminal: &falseVal,
			wantErr:    "no client specified and stdin is not a terminal; pass the client name or 'all' as an argument",
		},
		{
			name:      "all target skips already configured and installs json clients",
			args:      []string{"mcp", "install", "all", "--no-backup", "--json"},
			ghostPath: "/opt/bin/ghost",
			runner:    allConfiguredRunner,
			files:     allConfiguredFiles,
			// Cursor / Windsurf / Antigravity are JSON-config clients with no
			// CLI detection, so detection returns "not configured" and install
			// proceeds. Claude Code / Codex / Gemini / VS Code / Kiro CLI are
			// all pre-configured per the runner / fixtures above.
			wantStdout: "[\n" +
				"  {\n" +
				`    "client": "claude-code",` + "\n" +
				`    "status": "already configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "codex",` + "\n" +
				`    "status": "already configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "cursor",` + "\n" +
				`    "status": "installed",` + "\n" +
				`    "detail": "{{HOME}}/.cursor/mcp.json"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "gemini",` + "\n" +
				`    "status": "already configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "antigravity",` + "\n" +
				`    "status": "installed",` + "\n" +
				`    "detail": "{{HOME}}/.gemini/antigravity/mcp_config.json"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "kiro-cli",` + "\n" +
				`    "status": "already configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "vscode",` + "\n" +
				`    "status": "already configured"` + "\n" +
				"  },\n" +
				"  {\n" +
				`    "client": "windsurf",` + "\n" +
				`    "status": "installed",` + "\n" +
				`    "detail": "{{HOME}}/.codeium/windsurf/mcp_config.json"` + "\n" +
				"  }\n" +
				"]\n",
			after: assertCursorHasGhost,
		},
	}

	runMCPCmdTests(t, tests)
}

func TestMCPInstallSelectionOptions_DefaultSelection(t *testing.T) {
	opts := mcpInstallSelectionOptions()
	if !opts.selectedByDefault(mcpStatusNotConfigured, true) {
		t.Fatal("not-configured installed clients should be selected by default")
	}
	if opts.selectedByDefault(mcpStatusNotConfigured, false) {
		t.Fatal("not-configured clients that are not detected should not be selected by default")
	}
	if opts.selectedByDefault(mcpStatusConfigured, true) {
		t.Fatal("already-configured clients should not be selected by default")
	}
	if opts.selectedByDefault(mcpStatusError, true) {
		t.Fatal("clients with detection errors should not be selected by default")
	}
}
