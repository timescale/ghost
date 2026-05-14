package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectShellType(t *testing.T) {
	tests := map[string]string{
		"/bin/bash":          "bash",
		"/usr/local/bin/zsh": "zsh",
		"/opt/fish/fish":     "fish",
		"/bin/dash":          "",
		"":                   "",
	}
	for shell, want := range tests {
		t.Setenv("SHELL", shell)
		if got := DetectShellType(); got != want {
			t.Errorf("DetectShellType(SHELL=%q) = %q, want %q", shell, got, want)
		}
	}
}

func TestDetectShellRC_Zsh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")
	want := filepath.Join(home, ".zshrc")
	if got := DetectShellRC(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDetectShellRC_ZdotDir(t *testing.T) {
	home := t.TempDir()
	custom := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", custom)
	want := filepath.Join(custom, ".zshrc")
	if got := DetectShellRC(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDetectShellRC_Fish(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/usr/bin/fish")
	t.Setenv("XDG_CONFIG_HOME", "")
	want := filepath.Join(home, ".config", "fish", "config.fish")
	if got := DetectShellRC(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsInPath(t *testing.T) {
	t.Setenv("PATH", "/foo:/bar:/baz")
	if !IsInPath("/bar") {
		t.Errorf("expected /bar to be in PATH")
	}
	if IsInPath("/quux") {
		t.Errorf("expected /quux to be missing from PATH")
	}
	if IsInPath("") {
		t.Errorf("empty dir should never be in PATH")
	}
}

func TestCompletionSnippet(t *testing.T) {
	if got := CompletionSnippet("fish", "ghost"); got != "ghost completion fish | source" {
		t.Errorf("fish snippet mismatch: %q", got)
	}
	if got := CompletionSnippet("zsh", "ghost"); !strings.Contains(got, "source <(ghost completion zsh)") {
		t.Errorf("zsh snippet mismatch: %q", got)
	}
	if got := CompletionSnippet("bash", "ghost"); !strings.Contains(got, "source <(ghost completion bash)") {
		t.Errorf("bash snippet mismatch: %q", got)
	}
}

func TestShellRCNeedsCompinit(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")

	if !ShellRCNeedsCompinit("zsh", rc) {
		t.Errorf("missing zshrc should report needing compinit")
	}

	if err := os.WriteFile(rc, []byte("# bare\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ShellRCNeedsCompinit("zsh", rc) {
		t.Errorf("rc without compinit markers should need compinit")
	}

	if err := os.WriteFile(rc, []byte("source $ZSH/oh-my-zsh.sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ShellRCNeedsCompinit("zsh", rc) {
		t.Errorf("rc with oh-my-zsh should NOT need compinit")
	}

	if ShellRCNeedsCompinit("bash", rc) {
		t.Errorf("non-zsh shells should never need compinit")
	}
}

func TestShellRCMentions(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")

	mentioned, err := ShellRCMentions(rc, "ghost completion")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if mentioned {
		t.Errorf("missing file should not be reported as mentioning needle")
	}

	if err := os.WriteFile(rc, []byte("source <(ghost completion zsh)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mentioned, err = ShellRCMentions(rc, "ghost completion")
	if err != nil {
		t.Fatal(err)
	}
	if !mentioned {
		t.Errorf("file containing needle should be reported as mentioning it")
	}
}

func TestAppendPathToShellRC_Bash(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	installDir := "/opt/ghost/bin"
	if err := AppendPathToShellRC(rc, installDir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	want := "\n# Added by ghost init\nexport PATH=\"/opt/ghost/bin:$PATH\"\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAppendPathToShellRC_Fish(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, "fish", "config.fish")
	if err := AppendPathToShellRC(rc, "/opt/ghost/bin"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	want := "\n# Added by ghost init\nset -gx PATH /opt/ghost/bin $PATH\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAppendCompletionsToShellRC_ZshWithCompinit(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	if err := AppendCompletionsToShellRC(rc, "zsh", "ghost"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "autoload -Uz compinit && compinit -i") {
		t.Errorf("expected compinit block for empty zshrc: %q", got)
	}
	if !strings.Contains(string(got), "ghost completion zsh") {
		t.Errorf("expected ghost completion snippet: %q", got)
	}
}

func TestAppendCompletionsToShellRC_ZshWithExistingCompinit(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(rc, []byte("autoload -Uz compinit\ncompinit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendCompletionsToShellRC(rc, "zsh", "ghost"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "autoload -Uz compinit") != 1 {
		t.Errorf("should not duplicate compinit when already present: %q", got)
	}
}

func TestAppendCompletionsToShellRC_Bash(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	if err := AppendCompletionsToShellRC(rc, "bash", "ghost"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "compinit") {
		t.Errorf("bash rc should not include compinit: %q", got)
	}
	if !strings.Contains(string(got), "ghost completion bash") {
		t.Errorf("expected ghost completion snippet: %q", got)
	}
}
