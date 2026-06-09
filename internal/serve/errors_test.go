package serve

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/timescale/ghost/internal/common"
)

// TestHTTPStatusForFetchError covers the mapping from common database errors
// to HTTP status codes. It includes the wrapped/sentinel forms each branch is
// expected to recognize, plus the bad-gateway fallback for anything else.
func TestHTTPStatusForFetchError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "paused maps to conflict",
			err:        common.ErrPaused,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "not ready maps to conflict",
			err:        common.ErrNotReady,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "wrapped not ready still maps to conflict",
			err:        fmt.Errorf("failed to fetch schema: %w", common.ErrNotReady),
			wantStatus: http.StatusConflict,
		},
		{
			name:       "password not found maps to precondition failed",
			err:        common.ErrPasswordNotFound,
			wantStatus: http.StatusPreconditionFailed,
		},
		{
			name:       "schema not found maps to bad request",
			err:        &common.SchemaNotFoundError{Schema: "typo", Available: []string{"public"}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "wrapped schema not found still maps to bad request",
			err:        fmt.Errorf("failed to fetch schema: %w", &common.SchemaNotFoundError{Schema: "typo"}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "schema not found with listing failure still maps to bad request",
			err:        &common.SchemaNotFoundError{Schema: "typo", ListErr: errors.New("boom")},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid parameters exit code maps to bad request",
			err:        common.ExitWithCode(common.ExitInvalidParameters, errors.New("bad")),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "authentication exit code maps to unauthorized",
			err:        common.ExitWithCode(common.ExitAuthenticationError, errors.New("nope")),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "permission denied exit code maps to forbidden",
			err:        common.ExitWithCode(common.ExitPermissionDenied, errors.New("nope")),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "database not found exit code maps to not found",
			err:        common.ExitWithCode(common.ExitDatabaseNotFound, errors.New("gone")),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "timeout exit code maps to gateway timeout",
			err:        common.ExitWithCode(common.ExitTimeout, errors.New("slow")),
			wantStatus: http.StatusGatewayTimeout,
		},
		{
			name:       "unrecognized error maps to bad gateway",
			err:        errors.New("something upstream broke"),
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := httpStatusForFetchError(tc.err)
			if got != tc.wantStatus {
				t.Fatalf("httpStatusForFetchError() = %d, want %d", got, tc.wantStatus)
			}
		})
	}
}
