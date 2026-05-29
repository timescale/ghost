package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/timescale/ghost/internal/common"
)

type databaseListItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

func (s *Server) handleDatabases(w http.ResponseWriter, r *http.Request) {
	client, projectID, err := s.loadClient(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	resp, err := client.ListDatabasesWithResponse(r.Context(), projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("list databases: %v", err), http.StatusBadGateway)
		return
	}
	if resp.StatusCode() != http.StatusOK {
		http.Error(w, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSONDefault).Error(), resp.StatusCode())
		return
	}
	if resp.JSON200 == nil {
		http.Error(w, errors.New("empty response from API").Error(), http.StatusBadGateway)
		return
	}

	out := make([]databaseListItem, len(*resp.JSON200))
	for i, db := range *resp.JSON200 {
		out[i] = databaseListItem{
			ID:     db.Id,
			Name:   db.Name,
			Status: string(db.Status),
			Type:   string(db.Type),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}
