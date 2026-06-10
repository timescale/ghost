package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
	"github.com/timescale/ghost/internal/common"
)

// TestInit_NonInteractiveAllUnconfigured verifies that running `ghost init`
// without a TTY when nothing is configured returns an error listing every
// standalone command, with the --skip-if-configured hint at the bottom.
func TestInit_NonInteractiveAllUnconfigured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env-based completion detection is skipped on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("PATH", filepath.Join(home, "not-in-path"))
	t.Setenv("ZDOTDIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	withMCPClientCommandRunner(t, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	})

	result := runCommand(t, []string{"init"}, nil,
		withIsTerminal(false),
		withClientError(errors.New("not logged in")),
	)
	if result.err == nil {
		t.Fatalf("expected error, got nil\nstderr: %s", result.stderr)
	}
	expected := `ghost init requires an interactive terminal and cannot run here. To complete setup non-interactively, run these commands in order:
  ghost init path         # add ghost to your PATH
  ghost login             # authenticate (or use --api-key)
  ghost mcp install all   # install MCP server in all detected clients (or pass a specific client name)
  ghost init completions  # install shell completions in your shell rc file
Or pass --skip-if-configured to exit cleanly when everything is already set up`
	assertOutput(t, result.err.Error(), expected)
}

// TestInit_NonInteractiveOnlyLoginUnconfigured verifies that when most steps
// are already configured, the error only lists the remaining commands and
// omits the --skip-if-configured hint (since the flag was already passed).
func TestInit_NonInteractiveOnlyLoginUnconfigured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env-based completion detection is skipped on Windows")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	installDir := filepath.Dir(exe)
	t.Setenv("PATH", installDir)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("ZDOTDIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.WriteFile(filepath.Join(home, ".bashrc"),
		[]byte("source <(ghost completion bash)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cursorPath := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o755); err != nil {
		t.Fatal(err)
	}
	cursorCfg := `{"mcpServers":{"ghost":{"command":"/usr/local/bin/ghost","args":["mcp","start"]}}}`
	if err := os.WriteFile(cursorPath, []byte(cursorCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	withMCPClientCommandRunner(t, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	})

	result := runCommand(t, []string{"init", "--skip-if-configured"}, nil,
		withIsTerminal(false),
		withClientError(errors.New("not logged in")),
	)
	if result.err == nil {
		t.Fatalf("expected error, got nil\nstderr: %s", result.stderr)
	}
	expected := `ghost init requires an interactive terminal and cannot run here. To complete setup non-interactively, run these commands in order:
  ghost login  # authenticate (or use --api-key)`
	assertOutput(t, result.err.Error(), expected)
}

func TestInitCompletionSubcommandNonInteractive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell completions are not supported on Windows")
	}
	home := t.TempDir()
	rcPath := filepath.Join(home, ".bashrc")
	executablePath, err := getGhostExecutablePath()
	if err != nil {
		t.Fatalf("getGhostExecutablePath: %v", err)
	}

	result := runCommand(t, []string{"init", "completions"}, nil,
		withEnv("HOME", home),
		withEnv("SHELL", "/bin/bash"),
		withEnv("ZDOTDIR", ""),
		withEnv("XDG_CONFIG_HOME", ""),
		withIsTerminal(false),
	)
	if result.err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", result.err, result.stderr)
	}
	assertOutput(t, result.stdout, "")
	assertOutput(t, result.stderr, "Added bash completions to "+rcPath+".\nRestart your shell to apply changes.\n")

	gotRC, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotRC), common.CompletionSnippet("bash", executablePath)) {
		t.Fatalf("completion snippet not found in rc:\n%s", string(gotRC))
	}
}

func TestInitPathSubcommandNonInteractive(t *testing.T) {
	home := t.TempDir()
	executablePath, err := getGhostExecutablePath()
	if err != nil {
		t.Fatalf("getGhostExecutablePath: %v", err)
	}
	installDir := filepath.Dir(executablePath)
	rcPath := filepath.Join(home, ".bashrc")

	result := runCommand(t, []string{"init", "path"}, nil,
		withEnv("HOME", home),
		withEnv("SHELL", "/bin/bash"),
		withEnv("PATH", filepath.Join(home, "not-in-path")),
		withIsTerminal(false),
	)
	if result.err != nil {
		t.Fatalf("unexpected error: %v", result.err)
	}
	assertOutput(t, result.stdout, "")
	assertOutput(t, result.stderr, "Added "+installDir+" to PATH in "+rcPath+".\nRestart your shell to apply changes.\n")

	gotRC, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	assertOutput(t, string(gotRC), "\n# Added by ghost init\nexport PATH=\""+installDir+":$PATH\"\n")
}

func TestRunSelectedInitSteps_ConfiguresPathBeforeCompletions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("PATH", filepath.Join(home, "not-in-path"))
	t.Setenv("ZDOTDIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	executablePath, err := getGhostExecutablePath()
	if err != nil {
		t.Fatalf("getGhostExecutablePath: %v", err)
	}
	installDir := filepath.Dir(executablePath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := runSelectedInitSteps(cmd, &common.App{}, []int{int(stepPATH), int(stepCompletions)}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertOutput(t, stdout.String(), "")

	rcPath := filepath.Join(home, ".bashrc")
	gotRCBytes, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	gotRC := string(gotRCBytes)
	pathIndex := strings.Index(gotRC, "export PATH=\""+installDir+":$PATH\"")
	if pathIndex == -1 {
		t.Fatalf("PATH snippet not found in rc file:\n%s", gotRC)
	}
	completionSnippet := common.CompletionSnippet("bash", executablePath)
	completionIndex := strings.Index(gotRC, completionSnippet)
	if completionIndex == -1 {
		t.Fatalf("completion snippet %q not found in rc file:\n%s", completionSnippet, gotRC)
	}
	if completionIndex < pathIndex {
		t.Fatalf("completion snippet should appear after PATH snippet in rc file:\n%s", gotRC)
	}
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

	// MCP detection: write a JSON-config client (Cursor) so detectMCPState
	// reports at least one configured client. Cursor uses ~/.cursor/mcp.json
	// with MCPServersPathPrefix=/mcpServers.
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

	// Stub every external MCP-client CLI (claude / codex / gemini, etc.)
	// to behave as if the binary is not installed. Detection helpers treat
	// exec.ErrNotFound as "not configured", which keeps the test
	// hermetic regardless of what's actually on the developer's PATH.
	withMCPClientCommandRunner(t, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	})

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
