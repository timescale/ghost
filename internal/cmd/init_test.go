package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

func TestInit(t *testing.T) {
	// authInfoOK is a helper that mocks /auth/info returning a logged-in
	// user — used by tests that want detection to report "already logged in".
	authInfoOK := func(m *mock.MockClientWithResponsesInterface) {
		m.EXPECT().
			AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoType("user"),
					User: &api.UserInfo{Email: "you@example.com"},
				},
			}, nil).AnyTimes()
	}

	tests := []cmdTest{
		{
			name:    "non-interactive stdin returns error",
			args:    []string{"init"},
			setup:   authInfoOK,
			opts:    []runOption{withIsTerminal(false)},
			wantErr: "ghost init requires an interactive terminal; run it from a TTY",
		},
	}

	runCmdTests(t, tests)
}

func TestInit_SkipIfConfiguredAllConfigured(t *testing.T) {
	// This test sets up enough state for every detection to report
	// "configured", then verifies --skip-if-configured exits cleanly with
	// the expected hint on stderr.

	// Capture the executable path so we can ensure its directory is in
	// $PATH for the duration of the test. os.Executable() inside the test
	// binary points at the binary itself, so adding its dir to PATH makes
	// the PATH detection report "configured".
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	installDir := filepath.Dir(exe)
	t.Setenv("PATH", installDir)

	// Point HOME at a temp dir holding a shellrc that already sources
	// ghost completion. Also set SHELL so DetectShellType reports a known
	// value.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("ZDOTDIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	bashrc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(bashrc, []byte("source <(ghost completion bash)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// MCP detection: write the Claude Code config file with a ghost server
	// entry. The detect helper for Claude Code calls `claude mcp get` which
	// we cannot reliably stub from here without a custom runner; we accept
	// that MCP may not be "configured" in this environment and rely on a
	// JSON-config client instead. Cursor uses ~/.cursor/mcp.json with
	// MCPServersPathPrefix=/mcpServers.
	cursorPath := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// isGhostExecutableCommand keys off filepath.Base(command) == "ghost",
	// not the actual binary, so a synthetic path is fine here.
	cursorCfg := `{"mcpServers":{"ghost":{"command":"/usr/local/bin/ghost","args":["mcp","start"]}}}`
	if err := os.WriteFile(cursorPath, []byte(cursorCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	setup := func(m *mock.MockClientWithResponsesInterface) {
		m.EXPECT().
			AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoType("user"),
					User: &api.UserInfo{Email: "you@example.com"},
				},
			}, nil).AnyTimes()
	}

	result := runCommand(t, []string{"init", "--skip-if-configured"}, setup,
		withIsTerminal(false),
	)
	if result.err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "Ghost is already fully configured") {
		t.Fatalf("expected 'already fully configured' on stderr, got:\nstderr: %s", result.stderr)
	}
}
