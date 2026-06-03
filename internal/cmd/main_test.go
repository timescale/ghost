package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/google/go-cmp/cmp"
	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/util"
	"github.com/zalando/go-keyring"
	"go.uber.org/mock/gomock"
)

func TestMain(m *testing.M) {
	// Replace the system keyring with an in-memory mock so that tests
	// never read, write, or delete real credentials.
	keyring.MockInit()

	// Clear GHOST_EXPERIMENTAL so tests run with a consistent baseline.
	// Tests that need experimental features opt in with withEnv("GHOST_EXPERIMENTAL", "true").
	os.Unsetenv("GHOST_EXPERIMENTAL")

	os.Exit(m.Run())
}

type cmdResult struct {
	stdout string
	stderr string
	err    error
}

type runOption func(*runConfig)

type runConfig struct {
	stdin          io.Reader
	isTerminal     *bool // if set, overrides util.IsTerminal for this test
	ctx            context.Context
	envVars        map[string]string
	clientErr      error                                                                  // if set, the client factory returns this error (nil client)
	openBrowser    func(string) error                                                     // if set, overrides common.OpenBrowser for this test
	newGhostClient func(string, api.AuthMethod) (api.ClientWithResponsesInterface, error) // if set, overrides api.NewGhostClient
}

func withStdin(input string) runOption {
	return func(rc *runConfig) {
		rc.stdin = strings.NewReader(input)
	}
}

// withContext sets the context passed to cmd.ExecuteContext. Use this for
// commands that block until the context is cancelled (e.g. `ghost serve`):
// pass an already-cancelled context to exercise the command without leaving
// a server running for the duration of the test.
func withContext(ctx context.Context) runOption {
	return func(rc *runConfig) {
		rc.ctx = ctx
	}
}

// withIsTerminal overrides util.IsTerminal for the duration of the test.
// Use this with withStdin to simulate interactive terminal input.
func withIsTerminal(isTerminal bool) runOption {
	return func(rc *runConfig) {
		rc.isTerminal = &isTerminal
	}
}

func withEnv(key, value string) runOption {
	return func(rc *runConfig) {
		rc.envVars[key] = value
	}
}

// withClientError makes the client factory return the given error instead of a mock client.
// This simulates scenarios where the user is not logged in or credentials are invalid.
func withClientError(err error) runOption {
	return func(rc *runConfig) {
		rc.clientErr = err
	}
}

// withOpenBrowser overrides common.OpenBrowser for the duration of the test.
// By default, runCommand stubs OpenBrowser to return an error. Use this to
// simulate a successful browser open (pass a nil-returning func).
func withOpenBrowser(f func(string) error) runOption {
	return func(rc *runConfig) {
		rc.openBrowser = f
	}
}

// withNewGhostClient overrides api.NewGhostClient for the duration of the test.
// Use this to intercept client creation in flows like login that create their
// own API client (bypassing the mock injected by the client factory).
func withNewGhostClient(f func(string, api.AuthMethod) (api.ClientWithResponsesInterface, error)) runOption {
	return func(rc *runConfig) {
		rc.newGhostClient = f
	}
}

// runCommand builds the root command, injects a mock API client, and
// executes with the given args. Returns captured stdout, stderr, and
// any error from Execute.
func runCommand(
	t *testing.T,
	args []string,
	setupMock func(m *mock.MockClientWithResponsesInterface),
	opts ...runOption,
) cmdResult {
	t.Helper()

	rc := &runConfig{
		ctx:     context.Background(),
		envVars: map[string]string{},
	}
	for _, opt := range opts {
		opt(rc)
	}

	// Set and restore env vars
	for k, v := range rc.envVars {
		t.Setenv(k, v)
	}

	// Create mock
	ctrl := gomock.NewController(t)
	mockClient := mock.NewMockClientWithResponsesInterface(ctrl)
	if setupMock != nil {
		setupMock(mockClient)
	}

	// Build command and inject mock
	cmd, app, err := buildRootCmd()
	if err != nil {
		t.Fatalf("buildRootCmd failed: %v", err)
	}

	configDir := t.TempDir()
	app.SetClientFactory(func(ctx context.Context, cfg *config.Config) (api.ClientWithResponsesInterface, string, error) {
		if rc.clientErr != nil {
			return nil, "", rc.clientErr
		}
		return mockClient, "test-project", nil
	})

	// Capture output, stripping ANSI color/style sequences so tests can
	// use plain expected strings without embedded escape codes.
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&colorprofile.Writer{Forward: &stdout, Profile: colorprofile.NoTTY})
	cmd.SetErr(&colorprofile.Writer{Forward: &stderr, Profile: colorprofile.NoTTY})

	// Set stdin if provided
	if rc.stdin != nil {
		cmd.SetIn(rc.stdin)
	}

	// Override util.IsTerminal if requested
	if rc.isTerminal != nil {
		originalIsTerminal := util.IsTerminal
		val := *rc.isTerminal
		util.IsTerminal = func(w any) bool { return val }
		t.Cleanup(func() { util.IsTerminal = originalIsTerminal })
	}

	// Prevent browser opens in tests (default: return error).
	// Tests that need to simulate a successful browser open use withOpenBrowser.
	originalOpenBrowser := common.OpenBrowser
	if rc.openBrowser != nil {
		common.OpenBrowser = rc.openBrowser
	} else {
		common.OpenBrowser = func(url string) error {
			return errors.New("browser disabled in tests")
		}
	}
	t.Cleanup(func() { common.OpenBrowser = originalOpenBrowser })

	// Override api.NewGhostClient if requested (e.g. login tests)
	if rc.newGhostClient != nil {
		originalNewGhostClient := api.NewGhostClient
		api.NewGhostClient = rc.newGhostClient
		t.Cleanup(func() { api.NewGhostClient = originalNewGhostClient })
	}

	// Always include flags that prevent side effects in tests:
	// --config-dir: isolate from real config
	// --analytics=false: prevent analytics calls on mock
	// --version-check=false: prevent version check HTTP calls
	baseArgs := []string{
		"--config-dir", configDir,
		"--analytics=false",
		"--version-check=false",
	}
	cmd.SetArgs(append(baseArgs, args...))

	execErr := cmd.ExecuteContext(rc.ctx)

	return cmdResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    execErr,
	}
}

// httpResponse creates a minimal *http.Response with the given status code.
func httpResponse(statusCode int) *http.Response {
	return &http.Response{StatusCode: statusCode}
}

// sampleDatabase returns an api.Database with reasonable defaults.
// Use overrides to customize specific fields.
func sampleDatabase(overrides ...func(*api.Database)) api.Database {
	storageMib := 1024
	db := api.Database{
		Id:         "abc1234567",
		Name:       "mydb",
		Status:     api.DatabaseStatusRunning,
		Type:       api.DatabaseTypeStandard,
		Host:       "host.example.com",
		Port:       5432,
		StorageMib: &storageMib,
	}
	for _, o := range overrides {
		o(&db)
	}
	return db
}

// sampleDatabaseWithUsage returns an api.DatabaseWithUsage with reasonable defaults.
// Use overrides to customize specific fields.
func sampleDatabaseWithUsage(overrides ...func(*api.DatabaseWithUsage)) api.DatabaseWithUsage {
	storageMib := 1024
	db := api.DatabaseWithUsage{
		Id:             "abc1234567",
		Name:           "mydb",
		Status:         api.DatabaseStatusRunning,
		Type:           api.DatabaseTypeStandard,
		Host:           "host.example.com",
		Port:           5432,
		StorageMib:     &storageMib,
		ComputeMinutes: new(int64(90)),
	}
	for _, o := range overrides {
		o(&db)
	}
	return db
}

// validCtx is a gomock matcher that verifies a context.Context parameter is
// non-nil. Use this instead of gomock.Any() for context parameters. We only
// check non-nil (not cancellation state) because some flows use errgroup which
// cancels the derived context when one goroutine fails, and the other goroutine
// may receive the cancelled context legitimately.
var validCtx = gomock.Cond(func(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx != nil
})

// assertOutput checks that got exactly equals want, showing a unified diff on mismatch.
func assertOutput(t *testing.T, got, want string) {
	t.Helper()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}

// cmdTest is the standard test case struct for table-driven command tests.
type cmdTest struct {
	name       string
	args       []string
	setup      func(m *mock.MockClientWithResponsesInterface)
	opts       []runOption
	wantStdout string
	wantStderr string
	wantErr    string
}

// runCmdTests runs a slice of table-driven command tests using the standard
// assertion pattern: check wantErr, then wantStdout, then wantStderr.
//
// When wantErr is set and wantStderr is empty, the expected stderr is
// automatically derived from the error message (Cobra prints "Error: <msg>\n"
// to stderr for any error returned by RunE).
func runCmdTests(t *testing.T, tests []cmdTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCommand(t, tt.args, tt.setup, tt.opts...)

			if tt.wantErr != "" {
				if result.err == nil {
					t.Fatal("expected error, got nil")
				}
				assertOutput(t, result.err.Error(), tt.wantErr)
			} else if result.err != nil {
				t.Fatalf("unexpected error: %v", result.err)
			}

			assertOutput(t, result.stdout, tt.wantStdout)

			wantStderr := tt.wantStderr
			if wantStderr == "" && tt.wantErr != "" {
				// Cobra prints "Error: <msg>\n" to stderr for RunE errors
				wantStderr = "Error: " + tt.wantErr + "\n"
			}
			assertOutput(t, result.stderr, wantStderr)
		})
	}
}
