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

func newBootstrapHandler(projectID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(bootstrapResponse{
			ProjectID: projectID,
			Version:   config.Version,
		})
	})
}
