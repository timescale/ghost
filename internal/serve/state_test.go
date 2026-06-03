package serve

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// populatedState is reused across tests that exercise non-empty serialization.
var populatedState = serveState{
	SelectedDatabaseID: new("db-1"),
	EditorHeight:       new(240),
	EditorSQL:          new("select 1;"),
}

func TestStateStore_Load(t *testing.T) {
	tests := []struct {
		name string
		// setup runs before load. nil means "leave the temp dir empty so
		// the state file is missing".
		setup   func(t *testing.T, store *stateStore)
		wantErr bool
		check   func(t *testing.T, got serveState)
	}{
		{
			name: "missing file returns empty state",
			check: func(t *testing.T, got serveState) {
				if got != (serveState{}) {
					t.Errorf("load = %+v, want empty state", got)
				}
			},
		},
		{
			name: "round-trip via save restores all fields",
			setup: func(t *testing.T, store *stateStore) {
				if err := store.save(populatedState); err != nil {
					t.Fatalf("save: %v", err)
				}
			},
			check: func(t *testing.T, got serveState) {
				assertStringPtr(t, "SelectedDatabaseID", got.SelectedDatabaseID, "db-1")
				assertIntPtr(t, "EditorHeight", got.EditorHeight, 240)
				assertStringPtr(t, "EditorSQL", got.EditorSQL, "select 1;")
			},
		},
		{
			name: "omitted fields remain nil",
			setup: func(t *testing.T, store *stateStore) {
				writeStateFile(t, store, `{"selectedDatabaseId":"abc"}`)
			},
			check: func(t *testing.T, got serveState) {
				assertStringPtr(t, "SelectedDatabaseID", got.SelectedDatabaseID, "abc")
				if got.EditorHeight != nil {
					t.Errorf("EditorHeight = %v, want nil", *got.EditorHeight)
				}
				if got.EditorSQL != nil {
					t.Errorf("EditorSQL = %v, want nil", *got.EditorSQL)
				}
			},
		},
		{
			name: "invalid JSON returns error",
			setup: func(t *testing.T, store *stateStore) {
				writeStateFile(t, store, "{bad")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newStateStore(t.TempDir())
			if tc.setup != nil {
				tc.setup(t, store)
			}
			got, err := store.load()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestStateStore_SaveCreatesMissingConfigDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "config")
	if err := newStateStore(dir).save(serveState{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, stateFileName)); err != nil {
		t.Fatalf("expected state file to exist after save: %v", err)
	}
}

func TestStateStore_SaveWritesFileWith0600PermissionsUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics")
	}
	dir := t.TempDir()
	if err := newStateStore(dir).save(serveState{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, stateFileName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("perms = %v, want %v", mode, os.FileMode(0600))
	}
}

func TestStateHandlers(t *testing.T) {
	jsonGetHeaders := map[string]string{
		"Content-Type":  "application/json",
		"Cache-Control": "no-store",
	}

	tests := []struct {
		name        string
		method      string
		body        string
		presetState *serveState
		wantStatus  int
		wantHeaders map[string]string
		checkBody   func(t *testing.T, body []byte)
		checkStore  func(t *testing.T, store *stateStore)
	}{
		{
			name:        "GET returns empty state by default",
			method:      http.MethodGet,
			wantStatus:  http.StatusOK,
			wantHeaders: jsonGetHeaders,
			checkBody: func(t *testing.T, body []byte) {
				got := decodeServeState(t, body)
				if got != (serveState{}) {
					t.Errorf("body = %+v, want empty state", got)
				}
			},
		},
		{
			name:        "GET returns previously saved state",
			method:      http.MethodGet,
			presetState: &serveState{SelectedDatabaseID: new("db-9")},
			wantStatus:  http.StatusOK,
			wantHeaders: jsonGetHeaders,
			checkBody: func(t *testing.T, body []byte) {
				got := decodeServeState(t, body)
				assertStringPtr(t, "SelectedDatabaseID", got.SelectedDatabaseID, "db-9")
			},
		},
		{
			name:       "PUT persists body to store",
			method:     http.MethodPut,
			body:       `{"selectedDatabaseId":"db-7","editorHeight":300}`,
			wantStatus: http.StatusNoContent,
			checkStore: func(t *testing.T, store *stateStore) {
				got, err := store.load()
				if err != nil {
					t.Fatalf("load: %v", err)
				}
				assertStringPtr(t, "SelectedDatabaseID", got.SelectedDatabaseID, "db-7")
				assertIntPtr(t, "EditorHeight", got.EditorHeight, 300)
				if got.EditorSQL != nil {
					t.Errorf("EditorSQL = %v, want nil (not sent in body)", *got.EditorSQL)
				}
			},
		},
		{
			name:       "PUT rejects invalid JSON with 400",
			method:     http.MethodPut,
			body:       "{bad",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &Server{state: newStateStore(t.TempDir())}
			if tc.presetState != nil {
				if err := srv.state.save(*tc.presetState); err != nil {
					t.Fatalf("preset save: %v", err)
				}
			}

			var body io.Reader
			if tc.body != "" {
				body = bytes.NewReader([]byte(tc.body))
			}
			req := httptest.NewRequest(tc.method, "/api/state", body)
			rr := httptest.NewRecorder()
			switch tc.method {
			case http.MethodGet:
				srv.handleGetState(rr, req)
			case http.MethodPut:
				srv.handlePutState(rr, req)
			default:
				t.Fatalf("unsupported method %q", tc.method)
			}

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d\nbody: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			for header, want := range tc.wantHeaders {
				if got := rr.Header().Get(header); got != want {
					t.Errorf("header %s = %q, want %q", header, got, want)
				}
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rr.Body.Bytes())
			}
			if tc.checkStore != nil {
				tc.checkStore(t, srv.state)
			}
		})
	}
}

func writeStateFile(t *testing.T, store *stateStore, contents string) {
	t.Helper()
	if err := os.WriteFile(store.path, []byte(contents), 0600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
}

func decodeServeState(t *testing.T, body []byte) serveState {
	t.Helper()
	var got serveState
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v\nbody: %s", err, body)
	}
	return got
}

func assertStringPtr(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil, want %q", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s = %q, want %q", name, *got, want)
	}
}

func assertIntPtr(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil, want %d", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s = %d, want %d", name, *got, want)
	}
}
