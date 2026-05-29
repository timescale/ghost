package cmd

import (
	"errors"
	"testing"
)

func TestServeCmd(t *testing.T) {
	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"serve", "--no-open"},
			opts:    []runOption{withClientError(errors.New("authentication required: no credentials found"))},
			wantErr: "authentication required: no credentials found",
		},
	}
	runCmdTests(t, tests)
}
