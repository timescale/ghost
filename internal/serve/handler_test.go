package serve

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSchemaHandler_ParamValidation covers the query-param validation that the
// requiredQueryParam and boolQueryParam middleware perform before the schema
// handler runs (and before any client/database access). It drives the request
// through the full router so the middleware chain registered for /api/schema
// is exercised. The auth path and the FetchDatabaseSchema error mapping
// require a live database connection and are exercised by integration testing,
// not here.
// TestAgentEventsHandler_NoBridgeLiveness verifies that the agent SSE endpoint
// is served as a liveness stream even without an agent bridge (plain
// `ghost serve`): it responds 200 with the event-stream content type and holds
// the connection open until the client disconnects (context cancellation). The
// browser relies on this to detect when the backend goes away.
func TestAgentEventsHandler_NoBridgeLiveness(t *testing.T) {
	h := &Handler{logger: slog.Default()} // bridge is nil
	handler := h.Handler()

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/agent/events", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rr, req)
	}()

	// The handler should block (holding the stream open), not return early.
	select {
	case <-done:
		t.Fatal("handler returned before the client disconnected")
	case <-time.After(50 * time.Millisecond):
	}

	// Disconnect the client; the handler should then return promptly.
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after client disconnect")
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestSchemaHandler_ParamValidation(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "missing databaseId returns 400",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "blank databaseId returns 400",
			query:      "databaseId=",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-boolean internal returns 400",
			query:      "databaseId=db-1&internal=maybe",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "numeric internal is not a valid bool",
			query:      "databaseId=db-1&internal=2",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-boolean definitions returns 400",
			query:      "databaseId=db-1&definitions=maybe",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-boolean comments returns 400",
			query:      "databaseId=db-1&comments=maybe",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{logger: slog.Default()}
			handler := h.Handler()
			req := httptest.NewRequest(http.MethodGet, "/api/schema?"+tc.query, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}
