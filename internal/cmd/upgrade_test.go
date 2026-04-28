package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// startFakeReleasesServer starts an httptest.Server that mimics the release
// hosting used by scripts/install.sh (latest.txt + /releases/<ver>/<file>).
// Only the handlers we care about for a given test need to return data;
// anything else returns 404.
func startFakeReleasesServer(t *testing.T, latestVersion string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		if _, err := w.Write([]byte(latestVersion + "\n")); err != nil {
			t.Errorf("failed to write latest.txt response: %v", err)
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestUpgradeCmd(t *testing.T) {
	releasesServer := startFakeReleasesServer(t, "v99.99.99")

	tests := []cmdTest{
		{
			name: "rejects invalid --version",
			args: []string{"upgrade", "--version", "not-a-version"},
			opts: []runOption{
				withEnv("GHOST_RELEASES_URL", releasesServer.URL),
			},
			wantErr: `invalid version "not-a-version": must be a valid semver version (e.g. v1.2.3)`,
		},
		{
			name: "update alias rejects invalid --version",
			args: []string{"update", "--version", "1.2.3"},
			opts: []runOption{
				withEnv("GHOST_RELEASES_URL", releasesServer.URL),
			},
			wantErr: `invalid version "1.2.3": must be a valid semver version (e.g. v1.2.3)`,
		},
		{
			// config.Version is "dev" in tests, so every invocation without
			// --force exercises the dev-build guard.
			name: "refuses dev build without --force",
			args: []string{"upgrade"},
			opts: []runOption{
				withEnv("GHOST_RELEASES_URL", releasesServer.URL),
			},
			wantErr: "ghost is a local dev build, not a released version; re-run with --force to replace it with version v99.99.99",
		},
	}
	runCmdTests(t, tests)

	// Error from a network failure is non-deterministic (depends on the net
	// stack's exact wording), so we assert only the stable wrapping prefix
	// that runUpgrade adds.
	t.Run("fails when latest version cannot be fetched", func(t *testing.T) {
		result := runCommand(t, []string{"upgrade"}, nil,
			withEnv("GHOST_RELEASES_URL", "http://127.0.0.1:1"),
		)
		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		const wantPrefix = "failed to check for latest version: "
		if !strings.HasPrefix(result.err.Error(), wantPrefix) {
			t.Errorf("unexpected error: %v (want prefix %q)", result.err, wantPrefix)
		}
	})
}
