package writer

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve/api"
)

// EventWriter is responsible for taking a channel of [api.Event] messages
// returned from [Session.Events] and writing them to an [http.ResponseWriter]
// as newline-delimited JSON objects.
type EventWriter struct {
	w http.ResponseWriter
}

// NewEventWriter initializes a new [EventWriter].
func NewEventWriter(w http.ResponseWriter) *EventWriter {
	return &EventWriter{
		w: w,
	}
}

// Write takes a channel of [api.Result] messages (as returned from
// [Session.Events]) and writes them to the HTTP response as newline-delimited
// JSON objects.
func (ew *EventWriter) Write(ctx context.Context, events <-chan api.Event) {
	logger := log.FromContext(ctx)

	ew.w.Header().Set("Content-Type", "text/event-stream")
	ew.w.Header().Set("Content-Encoding", "gzip")
	ew.w.Header().Set("Cache-Control", "no-store, no-transform")
	ew.w.Header().Set("X-Accel-Buffering", "no")
	ew.w.WriteHeader(http.StatusOK)

	defer logger.Debug("Done writing results")
	defer drainChan(events)

	writer := newJSONWriter(ew.w)
	defer func() {
		if err := writer.Close(); err != nil {
			logger.Log(ctx, ErrLevel(ctx, err), "Error closing JSON writer", slog.Any("error", err))
		}
	}()

	for event := range events {
		switch event := event.(type) {
		case api.ConnectedEvent, api.ClosedEvent, api.ErrorEvent:
			if err := writer.Flush(event); err != nil {
				logger.Log(ctx, ErrLevel(ctx, err), "Error writing event", slog.Any("error", err))
				return
			}
		default:
			panic(fmt.Errorf("unexpected event type: %T", event))
		}
	}
}
