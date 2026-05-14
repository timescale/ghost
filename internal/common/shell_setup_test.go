package common

import (
	"os"
	"path/filepath"
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

func TestDetectShellRC(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		// setup configures the test-specific env vars and returns the
		// expected rc path. Called after HOME and SHELL are already set.
		setup func(t *testing.T, home string) string
	}{
		{
			name:  "zsh",
			shell: "/bin/zsh",
			setup: func(t *testing.T, home string) string {
				t.Setenv("ZDOTDIR", "")
				return filepath.Join(home, ".zshrc")
			},
		},
		{
			name:  "zsh with ZDOTDIR",
			shell: "/bin/zsh",
			setup: func(t *testing.T, home string) string {
				custom := t.TempDir()
				t.Setenv("ZDOTDIR", custom)
				return filepath.Join(custom, ".zshrc")
			},
		},
		{
			name:  "fish",
			shell: "/usr/bin/fish",
			setup: func(t *testing.T, home string) string {
				t.Setenv("XDG_CONFIG_HOME", "")
				return filepath.Join(home, ".config", "fish", "config.fish")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SHELL", tt.shell)
			want := tt.setup(t, home)
			if got := DetectShellRC(); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
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
	tests := []struct {
		shell string
		want  string
	}{
		{shell: "fish", want: "ghost completion fish | source"},
		{shell: "zsh", want: "command -v ghost >/dev/null 2>&1 && source <(ghost completion zsh)"},
		{shell: "bash", want: "command -v ghost >/dev/null 2>&1 && source <(ghost completion bash)"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			if got := CompletionSnippet(tt.shell, "ghost"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
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

func TestAppendPathToShellRC(t *testing.T) {
	tests := []struct {
		name       string
		rcFilename string
		want       string
	}{
		{
			name:       "bash",
			rcFilename: ".bashrc",
			want:       "\n# Added by ghost init\nexport PATH=\"/opt/ghost/bin:$PATH\"\n",
		},
		{
			name:       "fish",
			rcFilename: "fish/config.fish",
			want:       "\n# Added by ghost init\nset -gx PATH /opt/ghost/bin $PATH\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			rc := filepath.Join(dir, tt.rcFilename)
			if err := AppendPathToShellRC(rc, "/opt/ghost/bin"); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(rc)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppendCompletionsToShellRC(t *testing.T) {
	const zshSnippet = "command -v ghost >/dev/null 2>&1 && source <(ghost completion zsh)"
	const bashSnippet = "command -v ghost >/dev/null 2>&1 && source <(ghost completion bash)"

	tests := []struct {
		name        string
		shell       string
		rcFilename  string
		prefilledRC string
		want        string
	}{
		{
			name:       "zsh adds compinit block when missing",
			shell:      "zsh",
			rcFilename: ".zshrc",
			want:       "\n# Initialize zsh completions\nautoload -Uz compinit && compinit -i\n\n# Ghost shell completions\n" + zshSnippet + "\n",
		},
		{
			name:        "zsh skips compinit block when already present",
			shell:       "zsh",
			rcFilename:  ".zshrc",
			prefilledRC: "autoload -Uz compinit\ncompinit\n",
			want:        "autoload -Uz compinit\ncompinit\n\n# Ghost shell completions\n" + zshSnippet + "\n",
		},
		{
			name:       "bash never adds compinit",
			shell:      "bash",
			rcFilename: ".bashrc",
			want:       "\n# Ghost shell completions\n" + bashSnippet + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			rc := filepath.Join(dir, tt.rcFilename)
			if tt.prefilledRC != "" {
				if err := os.WriteFile(rc, []byte(tt.prefilledRC), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := AppendCompletionsToShellRC(rc, tt.shell, "ghost"); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(rc)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
