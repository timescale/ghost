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
// Arrow IPC stream of rows. We iterate the run's rows in-place, build
// record batches, and stream them out; when iteration finishes we signal
// Run.done so the executeQuery handler can emit its terminator.
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
	<-run.ready

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

	targets := run.columns.ScanTargets()
	for run.rows.Next() {
		if err := r.Context().Err(); err != nil {
			run.setError(&dbdriver.NormalizedError{Message: "request canceled", Source: "ghost", Cancel: true})
			break
		}
		if err := run.rows.Scan(targets...); err != nil {
			run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
			break
		}
		if err := rb.AppendRow(targets.Values()); err != nil {
			run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
			break
		}
		if rb.RecordRowCount() >= arrowBatchRows {
			if err := flushBatch(ipcWriter, rb, w); err != nil {
				run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
				break
			}
		}
	}
	if rb.RecordRowCount() > 0 {
		if err := flushBatch(ipcWriter, rb, w); err != nil && run.err == nil {
			run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
		}
	}

	if err := run.rows.Err(); err != nil && run.err == nil {
		// Defer to the driver's normalizer so PG errors carry code/hint/etc.
		if run.driver != nil {
			run.setError(run.driver.NormalizeError(run.queryCtx, err))
		} else {
			run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "postgres"})
		}
	}

	run.rowCount = rb.TotalRowCount()
	if rowsAffected, _ := run.rows.RowsAffected(r.Context()); rowsAffected != nil {
		run.rowsAffected = rowsAffected
	}
	run.closeDone()
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
