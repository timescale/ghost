package common

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// DetectShellType returns "bash", "zsh", "fish", or "" based on the SHELL
// environment variable.
func DetectShellType() string {
	shellName := filepath.Base(os.Getenv("SHELL"))
	switch shellName {
	case "bash", "zsh", "fish":
		return shellName
	}
	return ""
}

// DetectShellRC returns the path to the shell rc file that PATH and
// completion snippets should be appended to. Mirrors the heuristic in
// scripts/install.sh.
func DetectShellRC() string {
	home, _ := os.UserHomeDir()
	shellName := filepath.Base(os.Getenv("SHELL"))

	switch shellName {
	case "zsh":
		if zdotdir := os.Getenv("ZDOTDIR"); zdotdir != "" {
			return filepath.Join(zdotdir, ".zshrc")
		}
		return filepath.Join(home, ".zshrc")
	case "bash":
		// On macOS, login shells read .bash_profile, not .bashrc. Prefer
		// it when it already exists.
		bashProfile := filepath.Join(home, ".bash_profile")
		if runtime.GOOS == "darwin" {
			if _, err := os.Stat(bashProfile); err == nil {
				return bashProfile
			}
		}
		return filepath.Join(home, ".bashrc")
	case "fish":
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "fish", "config.fish")
	}

	// Unknown shell — guess based on OS and existing files.
	zshrc := filepath.Join(home, ".zshrc")
	if runtime.GOOS == "darwin" {
		return zshrc
	}
	if _, err := os.Stat(zshrc); err == nil {
		return zshrc
	}
	return filepath.Join(home, ".bashrc")
}

// IsInPath reports whether dir is an element of $PATH.
func IsInPath(dir string) bool {
	if dir == "" {
		return false
	}
	return slices.Contains(filepath.SplitList(os.Getenv("PATH")), dir)
}

// CompletionSnippet returns the shellrc line(s) that source Ghost's
// completion output for the given shell.
func CompletionSnippet(shell, binary string) string {
	quotedBinary := shellQuote(binary)
	switch shell {
	case "fish":
		return fmt.Sprintf("%s completion fish | source", quotedBinary)
	default:
		return fmt.Sprintf("command -v %s >/dev/null 2>&1 && source <(%s completion %s)", quotedBinary, quotedBinary, shell)
	}
}

var shellBareWordRE = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellQuote(value string) string {
	if value != "" && shellBareWordRE.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// compinitMarkerRE matches existing references to compinit or to common zsh
// frameworks that already initialize completions. When none of these markers
// are present, the completion snippet won't work without an explicit
// compinit call.
var compinitMarkerRE = regexp.MustCompile(`compinit|oh-my-zsh|prezto|zinit|antigen|zplug|zgenom`)

// ShellRCNeedsCompinit reports whether a zsh rc file is missing a compinit
// call. Returns false for non-zsh shells. Comment lines are ignored so a
// commented-out marker (e.g. `# compinit`) doesn't suppress the snippet.
func ShellRCNeedsCompinit(shell, rcPath string) bool {
	if shell != "zsh" {
		return false
	}
	data, err := readShellRCFileIfExists(rcPath)
	if err != nil {
		return true
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		if compinitMarkerRE.MatchString(line) {
			return false
		}
	}
	return true
}

// ShellRCMentions reports whether the file at rcPath contains needle. A
// missing file is treated as "not mentioned" with no error so callers can
// distinguish "doesn't reference" from "couldn't read".
func ShellRCMentions(rcPath, needle string) (bool, error) {
	data, err := readShellRCFileIfExists(rcPath)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), needle), nil
}

var ghostCompletionMentionRE = regexp.MustCompile(`(?:^|[^[:alnum:]_-])ghost(?:\.exe)?['"]?[[:space:]]+completion(?:[[:space:]]|$)`)

// ShellRCMentionsGhostCompletion reports whether the rc file appears to
// configure Ghost shell completions. It recognizes both the historical
// `ghost completion ...` form and the absolute-path form written by
// AppendCompletionsToShellRC. Comment lines are ignored so a commented-out
// snippet is not treated as configured.
func ShellRCMentionsGhostCompletion(rcPath string) (bool, error) {
	data, err := readShellRCFileIfExists(rcPath)
	if err != nil {
		return false, err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		if ghostCompletionMentionRE.MatchString(line) {
			return true, nil
		}
	}
	return false, nil
}

func readShellRCFileIfExists(rcPath string) ([]byte, error) {
	data, err := os.ReadFile(rcPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// AppendPathToShellRC adds a shell snippet to rcPath that prepends
// installDir to $PATH. For fish, emits an equivalent `set -gx PATH` snippet.
func AppendPathToShellRC(rcPath, installDir string) error {
	var snippet string
	if strings.HasSuffix(rcPath, "config.fish") {
		// Fish performs word-splitting on unquoted arguments, so an install
		// dir containing whitespace (e.g. "/Applications/Ghost CLI/bin")
		// would otherwise be split into separate PATH entries.
		snippet = fmt.Sprintf("\n# Added by ghost init\nset -gx PATH %s $PATH\n", shellQuote(installDir))
	} else {
		snippet = fmt.Sprintf("\n# Added by ghost init\nexport PATH=\"%s:$PATH\"\n", installDir)
	}
	return appendToShellRC(rcPath, snippet)
}

// AppendCompletionsToShellRC adds the completion snippet (plus a compinit
// block, when needed) to rcPath.
func AppendCompletionsToShellRC(rcPath, shell, binary string) error {
	var b strings.Builder
	if ShellRCNeedsCompinit(shell, rcPath) {
		b.WriteString("\n# Initialize zsh completions\n")
		b.WriteString("autoload -Uz compinit && compinit -i\n")
	}
	b.WriteString("\n# Ghost shell completions\n")
	b.WriteString(CompletionSnippet(shell, binary))
	b.WriteString("\n")
	return appendToShellRC(rcPath, b.String())
}

// appendToShellRC creates the parent directory if needed, ensures the file
// exists, and appends snippet to it.
func appendToShellRC(rcPath, snippet string) error {
	if err := os.MkdirAll(filepath.Dir(rcPath), 0o755); err != nil {
		return fmt.Errorf("failed to create rc directory: %w", err)
	}
	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", rcPath, err)
	}
	defer f.Close()
	if _, err := f.WriteString(snippet); err != nil {
		return fmt.Errorf("failed to write to %s: %w", rcPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", rcPath, err)
	}
	return nil
}
