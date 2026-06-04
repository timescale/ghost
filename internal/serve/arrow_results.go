package serve

import (
	"encoding/json"
	"net/http"

	"github.com/apache/arrow-go/v18/arrow/ipc"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

const arrowBatchRows = 1024

// handleArrowResults serves POST /api/arrowResults. The widget fires this
// immediately after seeing the executeQuery columns line and expects a raw
// Apache Arrow IPC stream of rows. Rows have already been scanned into
// run.bufferedRows by executeQuery (so we can pick the right result set out
// of a multi-statement run); we just convert them to Arrow record batches
// and stream them out. When we're done we signal Run.done so the
// executeQuery handler can emit its terminator.
func (s *Server) handleArrowResults(w http.ResponseWriter, r *http.Request) {
	var req arrowResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	run := s.runs.get(req.RunID)
	if run == nil {
		http.NotFound(w, r)
		return
	}
	// Wait for runQuery to finish buffering. Guard on the request context so a
	// stray/duplicate arrowResults POST for a run that errored before ready was
	// closed (e.g. the bufErr path in runQuery) doesn't block this handler
	// indefinitely.
	select {
	case <-run.ready:
	case <-r.Context().Done():
		return
	}

	rb, err := NewRecordBuilder(run.columns)
	if err != nil {
		http.Error(w, "arrow schema: "+err.Error(), http.StatusInternalServerError)
		run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
		run.closeDone()
		return
	}
	defer rb.Release()

	w.Header().Set("Content-Type", "application/vnd.apache.arrow.stream")
	w.Header().Set("Cache-Control", "no-store")

	ipcWriter := ipc.NewWriter(w, ipc.WithSchema(rb.Schema()))
	defer ipcWriter.Close()
	defer run.closeDone()

	for _, row := range run.bufferedRows {
		if err := r.Context().Err(); err != nil {
			run.setError(&dbdriver.NormalizedError{Message: "request canceled", Source: "ghost", Cancel: true})
			return
		}
		if err := rb.AppendRow(row); err != nil {
			run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
			return
		}
		if rb.RecordRowCount() >= arrowBatchRows {
			if err := flushBatch(ipcWriter, rb, w); err != nil {
				run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
				return
			}
		}
	}
	if rb.RecordRowCount() > 0 {
		if err := flushBatch(ipcWriter, rb, w); err != nil && run.err == nil {
			run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
		}
	}
}

func flushBatch(ipcWriter *ipc.Writer, rb *RecordBuilder, w http.ResponseWriter) error {
	batch := rb.NewRecordBatch()
	defer batch.Release()
	if err := ipcWriter.Write(batch); err != nil {
		return err
	}
	flushWriter(w)
	return nil
}
