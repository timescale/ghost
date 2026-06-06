package cmd

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestServeCmd(t *testing.T) {
	type serveCase struct {
		name string
		// preBindHost, if set, opens a listener on this host before invoking
		// `ghost serve`. Use this to force a deterministic bind failure (the
		// chosen port is substituted for the "%PORT%" placeholder in args).
		preBindHost string
		args        []string
		// opts builds the runOptions for this case. It receives *testing.T so
		// helpers like a fail-if-called OpenBrowser stub can use t.Fatal.
		opts func(t *testing.T) []runOption
		// Exactly one of wantErr / wantErrPrefix may be set. If neither is set,
		// the command is expected to succeed.
		wantErr        string
		wantErrPrefix  string
		stderrIncludes []string
		stderrExcludes []string
	}

	tests := []serveCase{
		{
			name: "not logged in",
			args: []string{"serve", "--no-open"},
			opts: func(t *testing.T) []runOption {
				return []runOption{withClientError(errors.New("authentication required: no credentials found"))}
			},
			wantErr: "authentication required: no credentials found",
		},
		{
			name:          "port already in use returns bind error",
			preBindHost:   "127.0.0.1",
			args:          []string{"serve", "--no-open", "--port", "%PORT%"},
			wantErrPrefix: "listen on 127.0.0.1:",
		},
		{
			name:           "non-loopback host emits warning before bind",
			preBindHost:    "0.0.0.0",
			args:           []string{"serve", "--no-open", "--host", "0.0.0.0", "--port", "%PORT%"},
			wantErrPrefix:  "listen on 0.0.0.0:",
			stderrIncludes: []string{"Binding to a non-loopback address exposes the SQL UI to your network"},
		},
		{
			name: "no-open skips browser",
			args: []string{"serve", "--no-open"},
			opts: func(t *testing.T) []runOption {
				// Cancel the context before runCommand executes so srv.Serve
				// returns immediately instead of blocking on a real listener.
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return []runOption{
					withContext(ctx),
					withOpenBrowser(func(string) error {
						t.Fatal("OpenBrowser must not be called when --no-open is set")
						return nil
					}),
				}
			},
			stderrIncludes: []string{
				"Listening url=http://127.0.0.1:",
				"Press Ctrl+C to stop",
			},
			stderrExcludes: []string{"Failed to open browser"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args
			if tc.preBindHost != "" {
				ln, err := net.Listen("tcp", tc.preBindHost+":0")
				if err != nil {
					t.Fatalf("pre-bind on %s: %v", tc.preBindHost, err)
				}
				defer ln.Close()
				port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
				args = slices.Clone(tc.args)
				for i, a := range args {
					if a == "%PORT%" {
						args[i] = port
					}
				}
			}

			var opts []runOption
			if tc.opts != nil {
				opts = tc.opts(t)
			}
			result := runCommand(t, args, nil, opts...)

			switch {
			case tc.wantErr != "":
				if result.err == nil {
					t.Fatal("expected error, got nil")
				}
				assertOutput(t, result.err.Error(), tc.wantErr)
			case tc.wantErrPrefix != "":
				if result.err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.HasPrefix(result.err.Error(), tc.wantErrPrefix) {
					t.Errorf("err = %q, want prefix %q", result.err.Error(), tc.wantErrPrefix)
				}
			default:
				if result.err != nil {
					t.Fatalf("unexpected error: %v", result.err)
				}
			}

			for _, want := range tc.stderrIncludes {
				if !strings.Contains(result.stderr, want) {
					t.Errorf("stderr missing %q:\n%s", want, result.stderr)
				}
			}
			for _, unwanted := range tc.stderrExcludes {
				if strings.Contains(result.stderr, unwanted) {
					t.Errorf("stderr should not contain %q:\n%s", unwanted, result.stderr)
				}
			}
		})
	}
}
