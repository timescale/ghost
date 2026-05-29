package serve

import (
	"encoding/json"
	"net/http"

	"github.com/timescale/ghost/internal/config"
)

type bootstrapResponse struct {
	ProjectID string `json:"projectId"`
	Version   string `json:"version"`
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	_, projectID, err := s.loadClient(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(bootstrapResponse{
		ProjectID: projectID,
		Version:   config.Version,
	})
}
