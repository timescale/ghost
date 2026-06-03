package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// serveState is the persisted UI state for `ghost serve`. Pointer fields let
// callers omit values they don't want to overwrite (encoded with omitempty).
type serveState struct {
	SelectedDatabaseID *string `json:"selectedDatabaseId,omitempty"`
	EditorHeight       *int    `json:"editorHeight,omitempty"`
	EditorSQL          *string `json:"editorSql,omitempty"`
}

const stateFileName = "serve-state.json"

// stateStore persists serveState to a JSON file in the user's config dir.
// Writes are atomic (temp file + rename) and serialized via a mutex so
// concurrent PUTs can't interleave.
type stateStore struct {
	path string
	lock sync.Mutex
}

func newStateStore(configDir string) *stateStore {
	return &stateStore{path: filepath.Join(configDir, stateFileName)}
}

func (s *stateStore) load() (serveState, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return serveState{}, nil
		}
		return serveState{}, fmt.Errorf("read state file: %w", err)
	}
	var state serveState
	if err := json.Unmarshal(data, &state); err != nil {
		return serveState{}, fmt.Errorf("parse state file: %w", err)
	}
	return state, nil
}

func (s *stateStore) save(state serveState) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".serve-state.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func (s *Server) handleGetState(w http.ResponseWriter, _ *http.Request) {
	state, err := s.state.load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(state)
}

func (s *Server) handlePutState(w http.ResponseWriter, r *http.Request) {
	var state serveState
	if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.state.save(state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
