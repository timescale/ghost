package serve

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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

// TestStateHandler_RoundTrip verifies that a PUT /api/state persists the full
// UI state and a subsequent GET returns it unchanged. It guards in particular
// against State fields being silently dropped on the round-trip: because a PUT
// replaces the stored state wholesale by unmarshaling into State, any field the
// web client sends but State omits is lost (this was a real regression for
// chartConfigHistory). Driving real JSON through the router exercises the
// unmarshalRequest middleware and the store's marshal/unmarshal path.
func TestStateHandler_RoundTrip(t *testing.T) {
	h := &Handler{
		logger: slog.Default(),
		store:  NewStore(t.TempDir(), slog.Default()),
	}
	handler := h.Handler()

	// A full snapshot that mirrors what the web client PUTs, including both
	// history lists. Each field must survive the round-trip.
	body := `{
		"selectedDatabaseId": "db-1",
		"editorSql": "select 1",
		"resultView": "chart",
		"chartConfig": "return {};",
		"queryHistory": [
			{"sql": "select 1", "ts": 1000, "success": true}
		],
		"chartConfigHistory": [
			{"config": "return {a:1};", "ts": 2000},
			{"config": "return {b:2};", "ts": 1500}
		]
	}`

	putReq := httptest.NewRequest(http.MethodPut, "/api/state", strings.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	handler.ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want %d\nbody: %s", putRR.Code, http.StatusNoContent, putRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d\nbody: %s", getRR.Code, http.StatusOK, getRR.Body.String())
	}

	// Responses are always gzip-encoded (see writeResponse).
	gz, err := gzip.NewReader(getRR.Body)
	if err != nil {
		t.Fatalf("failed to open gzip reader: %v", err)
	}
	defer gz.Close()
	var got GetStateResponse
	if err := json.NewDecoder(gz).Decode(&got); err != nil {
		t.Fatalf("failed to decode GET response: %v", err)
	}

	want := State{
		SelectedDatabaseID: "db-1",
		EditorSQL:          "select 1",
		ResultView:         "chart",
		ChartConfig:        "return {};",
		QueryHistory: []QueryHistoryEntry{
			{SQL: "select 1", Timestamp: 1000, Success: true},
		},
		ChartConfigHistory: []ChartConfigHistoryEntry{
			{Config: "return {a:1};", Timestamp: 2000},
			{Config: "return {b:2};", Timestamp: 1500},
		},
	}
	if diff := cmp.Diff(want, got.State); diff != "" {
		t.Fatalf("round-tripped state mismatch (-want +got):\n%s", diff)
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
