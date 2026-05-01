package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// mcpCmdTest is the table-driven test case struct for MCP command tests.
//
// In addition to the args / wantStdout / wantErr fields used by cmdTest, it
// has hooks for the file-system fixtures and command stubs required to
// exercise the install / status / uninstall subcommands without shelling
// out to real client binaries.
type mcpCmdTest struct {
	name string
	args []string

	// files are pre-created in the per-test temp HOME. Keys are paths
	// relative to that HOME (e.g. ".cursor/mcp.json"). The token "{{HOME}}"
	// in values is replaced with the absolute HOME path.
	files map[string]string

	// ghostPath stubs ghostExecutablePathFunc so the install command produces
	// deterministic output independent of the real binary location.
	ghostPath string

	// runner stubs runMCPClientCommand. Subcommand detection and CLI-based
	// install/uninstall invocations route through this stub.
	runner mcpClientCommandRunner

	// uninstallSelector stubs the interactive client selector used by
	// `ghost mcp uninstall` when no client argument is provided.
	uninstallSelector func(*cobra.Command) (string, error)

	// stdin / isTerminal exercise the interactive prompt code paths.
	stdin      string
	isTerminal *bool

	// wantStdout / wantStderr assert the command's output streams. The token
	// "{{HOME}}" is substituted with the per-test HOME directory.
	wantStdout string
	wantStderr string

	// wantErr asserts the result error's Error() string. When set and
	// wantStderr is empty, wantStderr is derived as "Error: <wantErr>\n".
	wantErr string

	// wantExitCode asserts the result error is an ExitWithCode error with the
	// matching code. Mutually exclusive with wantErr (assertExitCode requires
	// an empty Error() string).
	wantExitCode int

	// after runs additional assertions after the command finishes (e.g. for
	// verifying file-system side effects).
	after func(t *testing.T, homeDir string)
}

// runMCPCmdTests runs a slice of MCP command tests using the standard
// assertion pattern. Each test gets its own temp HOME directory.
func runMCPCmdTests(t *testing.T, tests []mcpCmdTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			expand := func(s string) string {
				return strings.ReplaceAll(s, "{{HOME}}", homeDir)
			}

			for relPath, content := range tt.files {
				writeTestFile(t, filepath.Join(homeDir, relPath), expand(content))
			}

			if tt.ghostPath != "" {
				withGhostExecutablePath(t, tt.ghostPath)
			}
			if tt.runner != nil {
				withMCPClientCommandRunner(t, tt.runner)
			}
			if tt.uninstallSelector != nil {
				original := uninstallTargetSelector
				uninstallTargetSelector = tt.uninstallSelector
				t.Cleanup(func() { uninstallTargetSelector = original })
			}

			opts := []runOption{withEnv("HOME", homeDir)}
			if tt.isTerminal != nil {
				opts = append(opts, withStdin(tt.stdin), withIsTerminal(*tt.isTerminal))
			} else if tt.stdin != "" {
				opts = append(opts, withStdin(tt.stdin))
			}

			result := runCommand(t, tt.args, nil, opts...)

			switch {
			case tt.wantExitCode != 0:
				assertExitCode(t, result.err, tt.wantExitCode)
			case tt.wantErr != "":
				if result.err == nil {
					t.Fatal("expected error, got nil")
				}
				assertOutput(t, result.err.Error(), tt.wantErr)
			default:
				if result.err != nil {
					t.Fatalf("unexpected error: %v", result.err)
				}
			}

			assertOutput(t, result.stdout, expand(tt.wantStdout))

			wantStderr := tt.wantStderr
			if wantStderr == "" && tt.wantErr != "" && tt.wantExitCode == 0 {
				wantStderr = "Error: " + tt.wantErr + "\n"
			}
			assertOutput(t, result.stderr, expand(wantStderr))

			if tt.after != nil {
				tt.after(t, homeDir)
			}
		})
	}
}

// withGhostExecutablePath overrides the ghost executable path resolver so the
// install command can produce deterministic output without depending on the
// real binary location.
func withGhostExecutablePath(t *testing.T, path string) {
	t.Helper()
	original := ghostExecutablePathFunc
	ghostExecutablePathFunc = func() (string, error) { return path, nil }
	t.Cleanup(func() { ghostExecutablePathFunc = original })
}

// withMCPClientCommandRunner overrides runMCPClientCommand for the duration
// of the test so that detection / CLI-based install or uninstall logic can be
// exercised without real client binaries on PATH.
func withMCPClientCommandRunner(t *testing.T, runner mcpClientCommandRunner) {
	t.Helper()
	original := runMCPClientCommand
	runMCPClientCommand = runner
	t.Cleanup(func() { runMCPClientCommand = original })
}

// assertMCPClientCommand asserts that the client CLI command (and its args)
// match the expected "command arg1 arg2 ..." form.
func assertMCPClientCommand(t *testing.T, command string, args []string, want string) {
	t.Helper()
	got := commandWithArgs(command, args)
	if got != want {
		t.Fatalf("command mismatch: got %q, want %q", got, want)
	}
}

// assertExitCode asserts that err is an ExitWithCode-style error with the
// matching code and an empty Error() string (the convention for
// common.ExitWithCode errors raised by `ghost mcp status` /
// `ghost mcp uninstall` when reporting overall outcomes).
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

// executableNotFoundError returns an exec.Error matching the one returned by
// the standard library when a command is not on PATH.
func executableNotFoundError(command string) error {
	return &exec.Error{Name: command, Err: exec.ErrNotFound}
}

// writeTestFile writes content to path, creating any missing parent directories.
func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}

// commandWithArgs joins a command and its args with spaces for assertion purposes.
func commandWithArgs(command string, args []string) string {
	return strings.Join(append([]string{command}, args...), " ")
}
