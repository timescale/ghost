package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/timescale/ghost/internal/cmd"
	"github.com/timescale/ghost/internal/mcp/query"
)

func main() {
	// sqlc plugin mode: while building query-tool metadata, sqlc re-invokes
	// this executable as a subprocess with SQLC_VERSION set (sqlc always sets
	// it when calling process plugins). See query.RunPlugin.
	if os.Getenv("SQLC_VERSION") != "" {
		if err := query.RunPlugin(); err != nil {
			fmt.Fprintf(os.Stderr, "sqlc plugin failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// sqlc runner mode: the MCP server spawns the binary in this mode to run
	// sqlc out-of-process (see query.RunSqlc for why). sqlc may call
	// os.Exit itself, so this branch is terminal.
	if cfg := os.Getenv(query.SqlcConfigEnvVar); cfg != "" {
		os.Exit(query.RunSqlcDirect(cfg))
	}

	if err := run(); err != nil {
		// Check if it's a custom exit code error
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func run() (err error) {
	ctx, cancel := notifyContext(context.Background())
	defer func() {
		cancel()
		if r := recover(); r != nil {
			err = errors.Join(err, fmt.Errorf("panic: %v", r))
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
		}
	}()
	return cmd.Execute(ctx)
}

// noifyContext sets up graceful shutdown handling and returns a context and
// cleanup function. This is nearly identical to [signal.NotifyContext], except
// that it also restores the default signal handling behavior.
func notifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			signal.Stop(sigChan) // Restore default signal handling behavior
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, func() {
		cancel()
		signal.Stop(sigChan)
	}
}
