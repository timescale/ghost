package serve

import (
	"encoding/json"
	"net/http"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/ipc"

	"github.com/timescale/ghost/internal/serve/dbdriver"
)

// handleArrowResults serves POST /api/arrowResults. The widget fires this
// immediately after seeing the executeQuery columns line and expects a raw
// Apache Arrow IPC stream of rows. The query goroutine (streamQuery) streams
// scanned rows over run.rows; we convert them to Arrow record batches and
// write them straight to the response. Backpressure on run.rows keeps memory
// bounded and ensures a fast time-to-first-byte for large result sets. When
// we're done we signal run.done so executeQuery can emit its terminator.
func (s *Server) handleArrowResults(w http.ResponseWriter, r *http.Request) {
	var req arrowResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	run := s.runs.get(req.RunID)
	if run == nil {
		http.NotFound(w, r)
		return
	}

	// Only one caller may drain run.rows. Reject concurrent/duplicate fetches
	// (mirrors the upstream single-reader pipe design). Without this guard a second
	// caller would silently receive a truncated stream.
	if !run.arrowStarted.CompareAndSwap(false, true) {
		writeJSONError(w, http.StatusConflict, "arrow results are already being streamed for this run")
		return
	}

	// Wait for streamQuery to publish columns. Guard on the request context so
	// a stray request for a run that errored before columns were produced
	// doesn't block this handler indefinitely.
	select {
	case <-run.ready:
	case <-r.Context().Done():
		return
	}

	rb, err := NewRecordBuilder(run.columns)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "arrow schema: "+err.Error())
		run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
		run.cancelQuery()
		run.closeDone()
		return
	}
	defer rb.Release()

	w.Header().Set("Content-Type", "application/vnd.apache.arrow.stream")
	w.Header().Set("Cache-Control", "no-store")

	ipcWriter := ipc.NewWriter(w, ipc.WithSchema(rb.Schema()))
	defer ipcWriter.Close()
	defer run.closeDone()

	// batchRows is the target row count for the next record batch. It starts
	// small (fast first byte) and is recomputed after each flush to track a
	// target byte size, matching the upstream adaptive batching design.
	batchRows := int64(initialRecordRowCount)
	for row := range run.rows {
		if err := r.Context().Err(); err != nil {
			run.setError(&dbdriver.NormalizedError{Message: "request canceled", Source: "ghost", Cancel: true})
			run.cancelQuery()
			return
		}
		if err := rb.AppendRow(row); err != nil {
			run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
			run.cancelQuery()
			return
		}
		if rb.RecordRowCount() >= batchRows {
			newTarget, err := flushBatch(ipcWriter, rb, w, batchRows)
			if err != nil {
				run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
				run.cancelQuery()
				return
			}
			batchRows = newTarget
		}
	}
	if rb.RecordRowCount() > 0 {
		if _, err := flushBatch(ipcWriter, rb, w, batchRows); err != nil && run.err == nil {
			run.setError(&dbdriver.NormalizedError{Message: err.Error(), Source: "ghost"})
		}
	}
}

// flushBatch finalizes the in-progress record batch, writes it to the IPC
// stream, flushes it to the client, and returns the target row count for the
// next batch (recomputed from the batch just written).
func flushBatch(ipcWriter *ipc.Writer, rb *RecordBuilder, w http.ResponseWriter, oldRowCount int64) (int64, error) {
	batch := rb.NewRecordBatch()
	defer batch.Release()
	newRowCount := newRecordRowCount(batch, oldRowCount)
	if err := ipcWriter.Write(batch); err != nil {
		return oldRowCount, err
	}
	flushWriter(w)
	return newRowCount, nil
}

const (
	// initialRecordRowCount is the number of rows in the first record batch.
	// Kept small so the user sees the first rows quickly.
	initialRecordRowCount = 100

	// maxRecordRowCount caps the number of rows in any record batch.
	maxRecordRowCount = 10000

	// minRecordRowCount is the floor for the number of rows in a record batch.
	minRecordRowCount = 5

	// targetRecordBytes is the target serialized size of a record batch. Any
	// given batch can overshoot or undershoot; the next batch's row count is
	// adjusted to home in on this target.
	targetRecordBytes = 5 * 1024 * 1024 // 5 MiB
)

// newRecordRowCount computes the ideal number of rows for the next record
// batch from the average bytes-per-row of the last batch, clamped to sane
// bounds. This adaptive sizing keeps memory spikes small and
// time-to-first-byte fast regardless of row width.
func newRecordRowCount(batch arrow.RecordBatch, oldRowCount int64) int64 {
	recordBytes := recordSizeBytes(batch)
	if recordBytes == 0 || oldRowCount == 0 {
		return oldRowCount
	}
	bytesPerRow := recordBytes / uint64(oldRowCount)
	if bytesPerRow == 0 {
		bytesPerRow = 1
	}
	newRowCount := int64(targetRecordBytes / bytesPerRow)

	// Clamp between the min and max, and limit sudden growth to 2x the
	// previous count (in case the last batch was not a representative sample).
	newRowCount = min(newRowCount, oldRowCount*2, maxRecordRowCount)
	newRowCount = max(newRowCount, minRecordRowCount)
	return newRowCount
}

func recordSizeBytes(batch arrow.RecordBatch) uint64 {
	var size uint64
	for _, col := range batch.Columns() {
		size += col.Data().SizeInBytes()
	}
	return size
}
