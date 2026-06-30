package serve

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve/api"
)

// agentEventsHandler serves GET /api/agent/events as a Server-Sent Events
// stream. The stream doubles as a backend-liveness signal: the browser treats
// an open stream as "connected" and a dropped stream as "backend gone", so it
// is served even when there is no agent bridge (plain `ghost serve`). When a
// bridge is present, the connecting tab is registered with it and additionally
// receives "status" events (whether it is the active controlling tab) and
// "command" events (work dispatched by MCP tools). The stream stays open until
// the client disconnects or the server shuts down.
func (h *Handler) agentEventsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// The default http.ResponseWriter implements http.Flusher and we never wrap
	// it with one that doesn't, so a failed assertion is a programmer error —
	// panic rather than handle it (matching writer.NewFlushWriter).
	flusher := w.(http.Flusher)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Without a bridge there are no agent events to send; just hold the
	// connection open as a liveness signal until the client or server goes away.
	if h.bridge == nil {
		<-ctx.Done()
		return
	}

	client := h.bridge.addClient()
	defer h.bridge.removeClient(client)
	logger.Debug("Agent client connected", slog.String("clientId", client.id))

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Agent client disconnected", slog.String("clientId", client.id))
			return
		case event := <-client.events:
			data, err := json.Marshal(event)
			if err != nil {
				logger.Error("Error marshaling agent event", slog.Any("error", err))
				continue
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				logger.Debug("Error writing agent event", slog.Any("error", err))
				return
			}
			if _, err := w.Write(data); err != nil {
				logger.Debug("Error writing agent event", slog.Any("error", err))
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				logger.Debug("Error writing agent event", slog.Any("error", err))
				return
			}
			flusher.Flush()
		}
	}
}

// agentResponseType identifies what a browser is posting back to
// POST /api/agent/respond for an in-flight command.
type agentResponseType string

const (
	// agentResponseHeartbeat keeps the request alive while the browser works.
	agentResponseHeartbeat agentResponseType = "heartbeat"
	// agentResponseResult delivers a successful command result.
	agentResponseResult agentResponseType = "result"
	// agentResponseError reports that the command failed in the browser.
	agentResponseError agentResponseType = "error"
)

// AgentRespondRequest is the request body of POST /api/agent/respond. The
// browser posts heartbeats, command results, and errors back over this
// endpoint, keyed by the originating client and request IDs.
type AgentRespondRequest struct {
	ClientID  string            `json:"clientId"`
	RequestID string            `json:"requestId"`
	Type      agentResponseType `json:"type"`
	Data      json.RawMessage   `json:"data,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// Validate returns an error if a required field is missing.
func (r AgentRespondRequest) Validate() error {
	if r.ClientID == "" {
		return &RequiredFieldError{Field: "clientId"}
	}
	if r.RequestID == "" {
		return &RequiredFieldError{Field: "requestId"}
	}
	if r.Type == "" {
		return &RequiredFieldError{Field: "type"}
	}
	return nil
}

// AgentRespondResponse is the response body of POST /api/agent/respond.
type AgentRespondResponse struct {
	Success bool `json:"success"`
}

func (h *Handler) agentRespondHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	req := requestFromContext(ctx).(*AgentRespondRequest)

	if h.bridge == nil {
		writeError(w, http.StatusNotFound, api.ErrNotFound, logger)
		return
	}

	if err := h.bridge.deliver(req.ClientID, req.RequestID, req.Type, req.Data, req.Error); err != nil {
		// A stale or mismatched response is expected (e.g. after a takeover or
		// timeout) and is not a server error.
		logger.Debug("Discarding agent response", slog.Any("error", err))
		writeError(w, http.StatusConflict, err, logger)
		return
	}

	writeJSON(w, http.StatusOK, AgentRespondResponse{Success: true}, logger)
}

// AgentActivateRequest is the request body of POST /api/agent/activate. A
// browser tab posts its own client ID to become the active controlling tab
// (the "take over" action).
type AgentActivateRequest struct {
	ClientID string `json:"clientId"`
}

// Validate returns an error if a required field is missing.
func (r AgentActivateRequest) Validate() error {
	if r.ClientID == "" {
		return &RequiredFieldError{Field: "clientId"}
	}
	return nil
}

// AgentActivateResponse is the response body of POST /api/agent/activate.
type AgentActivateResponse struct {
	Success bool `json:"success"`
}

func (h *Handler) agentActivateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)
	req := requestFromContext(ctx).(*AgentActivateRequest)

	if h.bridge == nil {
		writeError(w, http.StatusNotFound, api.ErrNotFound, logger)
		return
	}

	if !h.bridge.Activate(req.ClientID) {
		writeError(w, http.StatusNotFound, api.ErrNotFound, logger)
		return
	}

	writeJSON(w, http.StatusOK, AgentActivateResponse{Success: true}, logger)
}
