package serve

import (
	"encoding/json"
	"net/http"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

// handleCancelRun serves POST /api/cancelRun. The widget rarely uses this
// path (it prefers AbortController on the executeQuery request), but we
// support it: looking up the run by ID and triggering its queryCtx cancel,
// which routes through pgConn.CancelRequest server-side.
func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	var req cancelQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	run := s.runs.get(req.RunID)
	if run == nil {
		http.NotFound(w, r)
		return
	}
	run.setError(&dbdriver.NormalizedError{Message: "query canceled by user", Source: "ghost", Cancel: true})
	run.cancelQuery()
	w.WriteHeader(http.StatusNoContent)
}
