package serve

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/log"
)

// schemaHandler serves GET /api/schema?databaseId=…&schema=…&internal=true&definitions=true.
// Returns the database schema as JSON. Opens a short-lived pgx connection
// per request via common.FetchDatabaseSchema — same path used by the CLI's
// `ghost schema` command, so the two stay in sync automatically.
func (h *Handler) schemaHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	databaseRef := r.URL.Query().Get("databaseId")
	if databaseRef == "" {
		writeError(w, http.StatusBadRequest, errors.New("databaseId query param is required"), logger)
		return
	}

	schemaName := r.URL.Query().Get("schema")

	includeInternal := false
	if v := r.URL.Query().Get("internal"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("internal must be a boolean"), logger)
			return
		}
		includeInternal = parsed
	}

	includeDefinitions := false
	if v := r.URL.Query().Get("definitions"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("definitions must be a boolean"), logger)
			return
		}
		includeDefinitions = parsed
	}

	client, projectID, err := h.loadClient(ctx)
	if err != nil {
		logger.Warn("Error loading client", slog.Any("error", err))
		writeError(w, http.StatusUnauthorized, err, logger)
		return
	}

	schema, err := common.FetchDatabaseSchema(ctx, common.FetchDatabaseSchemaArgs{
		Client:             client,
		ProjectID:          projectID,
		DatabaseRef:        databaseRef,
		Schema:             schemaName,
		IncludeInternal:    includeInternal,
		IncludeDefinitions: includeDefinitions,
	})
	if err != nil {
		status := httpStatusForFetchError(err)
		// Client errors (4xx) — e.g. a mistyped ?schema= — are expected and
		// shouldn't be logged as server-side errors.
		if status >= http.StatusInternalServerError {
			logger.Error("Error fetching database schema", slog.Any("error", err))
		} else {
			logger.Warn("Could not fetch database schema", slog.Any("error", err))
		}
		writeError(w, status, err, logger)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, schema, logger)
}
