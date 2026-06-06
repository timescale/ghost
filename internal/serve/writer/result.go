package writer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve/api"
)

// ResultWriter is responsible for taking a channel of [api.Result] messages
// returned from [Session.Query] and writing them to an [http.ResponseWriter]
// as newline-delimited JSON objects (or otherwise handling them - e.g. by
// converting them to [arrow.RecordBatch] batches and writing them to the
// destinations/formats specified in the provided [Outputs]).
type ResultWriter struct {
	outputs Outputs
	w       http.ResponseWriter
}

// NewResultWriter initializes a new [ResultWriter].
func NewResultWriter(outputs Outputs, w http.ResponseWriter) *ResultWriter {
	return &ResultWriter{
		outputs: outputs,
		w:       w,
	}
}

// statusResultInterval is the interval of time after which an [api.StatusResult]
// message is written if the connection is otherwise idle (i.e. if no other
// [api.Result] messages have been received from the results channel).
const statusResultInterval = 15 * time.Second

// Write takes a channel of [api.Result] messages (as returned from
// [Session.Query]) and writes them to the HTTP response as newline-delimited
// JSON objects. If the [Run] is configured to output results to other
// destinations/formats (via the provided [Outputs]), it does not write
// [api.RowResult] messages to the response, but instead converts them to
// [arrow.RecordBatch] batches and writes them to the requested
// formats/destinations.
func (rw *ResultWriter) Write(ctx context.Context, results <-chan api.Result) {
	logger := log.FromContext(ctx)

	rw.w.Header().Set("Content-Type", "text/event-stream")
	rw.w.Header().Set("Content-Encoding", "gzip")
	rw.w.Header().Set("Cache-Control", "no-store, no-transform")
	rw.w.Header().Set("X-Accel-Buffering", "no")
	rw.w.WriteHeader(http.StatusOK)

	defer logger.Debug("Done writing results")
	defer drainChan(results)

	writer := newJSONWriter(rw.w)
	defer func() {
		if err := writer.Close(); err != nil {
			logger.Log(ctx, ErrLevel(ctx, err), "Error closing JSON writer", slog.Any("error", err))
		}
	}()

	ticker := time.NewTicker(statusResultInterval)
	defer ticker.Stop()

	handler := rw.newResultHandler(ctx, writer, ticker)
	defer handler.Close()

	for {
		result := rw.getResult(results, ticker)
		if result == nil {
			return
		}

		if err := handler.HandleResult(ctx, result); err != nil {
			// If the error was a [WriteError], do not attempt to write
			// anything else to the response - just log the error and quit.
			var writeErr *WriteError
			if errors.As(err, &writeErr) {
				logger.Log(ctx, ErrLevel(ctx, err), "Error writing result", slog.Any("error", err))
				return
			}

			// For any other error type, write an error message to the response
			// so that the caller knows something went wrong.
			logger.Error("Error handling result", slog.Any("error", err))
			if err := writer.WriteError(api.ErrInternalServer); err != nil {
				logger.Log(ctx, ErrLevel(ctx, err), "Error writing error result", slog.Any("error", err))
			}
			return
		}
	}
}

// getResult reads an [api.Result] off of the results channel and returns it.
// If the channel has been closed, it returns nil. If no results have been read
// for the period of time specified by statusResultInterval, it returns an
// [api.StatusResult], which should immediately be written to the response and
// flushed to indicate that the query is still pending (and to help keep the
// connection from being forcibly killed due to an idle connection timeout).
func (rw *ResultWriter) getResult(results <-chan api.Result, ticker *time.Ticker) api.Result {
	select {
	case result, ok := <-results:
		if !ok {
			return nil
		}
		ticker.Reset(statusResultInterval)
		return result
	case <-ticker.C:
		return api.StatusResult{
			Status: api.StatusPending,
		}
	}
}

// resultHandler is an interface for a type that is capable of handling an
// [api.Result] message. This includes determining how and whether to write it
// to the response, as well as performing any special logic necessary for that
// result type. It currently has two implementations: one for when results are
// being returned in newline-delimited JSON format, and one for when results
// are being converted to [arrow.RecordBatch] batches and written to the
// formats/destinations configured in the provided [Outputs].
type resultHandler interface {
	HandleResult(ctx context.Context, result api.Result) error
	Close()
}

func (rw *ResultWriter) newResultHandler(ctx context.Context, writer *jsonWriter, ticker *time.Ticker) resultHandler {
	if len(rw.outputs) > 0 {
		return rw.newArrowResultHandler(ctx, writer, ticker)
	}
	return rw.newJSONResultHandler(writer)
}

type jsonResultHandler struct {
	writer *jsonWriter
}

func (rw *ResultWriter) newJSONResultHandler(writer *jsonWriter) *jsonResultHandler {
	return &jsonResultHandler{
		writer: writer,
	}
}

func (h *jsonResultHandler) HandleResult(ctx context.Context, result api.Result) error {
	switch result := result.(type) {
	case api.ColumnResult, api.RowResult, api.SuccessResult, api.ErrorResult:
		return h.writer.Write(result)
	case api.StatusResult:
		return h.writer.Flush(result)
	default:
		panic(fmt.Errorf("unexpected result type: %T", result))
	}
}

func (h *jsonResultHandler) Close() {}

const (
	// The number of rows in the first Arrow record batch.
	initialRecordRowCount = 100

	// The maximum number of rows in each Arrow record batch.
	maxRecordRowCount = 10000

	// The minimum number of rows in each Arrow record batch.
	minRecordRowCount = 5

	// The target size in bytes for an Arrow record batch. Note that any given
	// record batch can overshoot or undershoot this value, but the number of
	// rows in the next batch will be adjusted accordingly to ideally hit the
	// target.
	targetRecordBytes = 5 * 1024 * 1024 // 5 MiB
)

type arrowResultHandler struct {
	outputs        Outputs
	writer         *jsonWriter
	ticker         *time.Ticker
	recordRowCount int64

	builder     *RecordBuilder
	recordChans []chan<- arrow.RecordBatch
	wg          sync.WaitGroup
}

func (rw *ResultWriter) newArrowResultHandler(ctx context.Context, writer *jsonWriter, ticker *time.Ticker) *arrowResultHandler {
	return &arrowResultHandler{
		outputs:        rw.outputs,
		writer:         writer,
		ticker:         ticker,
		recordRowCount: initialRecordRowCount,
	}
}

func (h *arrowResultHandler) HandleResult(ctx context.Context, result api.Result) error {
	switch result := result.(type) {
	case api.ColumnResult:
		builder, err := NewRecordBuilder(result.Columns)
		if err != nil {
			return fmt.Errorf("error creating arrow record builder: %w", err)
		}
		h.builder = builder
		schema := h.builder.Schema()

		// Initialize record handlers
		for _, output := range h.outputs {
			records := h.newRecordHandler(ctx, output, schema)
			h.recordChans = append(h.recordChans, records)
		}

		// Flush result so that caller knows it can safely make a
		// request to the GET /user/:userId/run/:runId/arrow endpoint.
		return h.writer.Flush(result)
	case api.RowResult:
		if err := h.builder.AppendRow(result.Row); err != nil {
			return fmt.Errorf("error appending row to arrow record builder: %w", err)
		}
		if h.builder.RecordRowCount() == h.recordRowCount {
			record := h.builder.NewRecord()
			h.recordRowCount = newRecordRowCount(ctx, record, h.recordRowCount)
			return h.send(ctx, record)
		}
		return nil
	case api.StatusResult:
		// Flush result to ensure caller gets it in a timely manner, since the
		// send call below could theoretically block until the next status tick.
		if err := h.writer.Flush(result); err != nil {
			return err
		}

		// Send a batch here, even if empty, to ensure the connection remains
		// active and isn't forcibly killed due to an idle connection timeout.
		if h.builder != nil {
			return h.send(ctx, h.builder.NewRecord())
		}
		return nil
	case api.SuccessResult, api.ErrorResult:
		if h.builder != nil {
			// Always send a record here, even if it's empty. This ensures
			// that the code always blocks here until the arrow stream is
			// fetched, even in the case where the query returned no data.
			// This prevents the caller from getting an unexpected error
			// when trying to fetch the arrow results.
			if err := h.send(ctx, h.builder.NewRecord()); err != nil {
				return err
			}
		}
		return h.writer.Flush(result)
	default:
		panic(fmt.Errorf("unexpected result type: %T", result))
	}
}

func (h *arrowResultHandler) newRecordHandler(ctx context.Context, output Output, schema *arrow.Schema) chan<- arrow.RecordBatch {
	ctx, logger := log.NewContext(ctx,
		log.FromContext(ctx).With(
			slog.String("format", string(output.Format)),
			slog.String("destination", string(output.Destination)),
			slog.String("compression", string(output.Compression)),
		),
	)

	records := make(chan arrow.RecordBatch)

	h.wg.Add(1)
	go func(records <-chan arrow.RecordBatch) {
		defer h.wg.Done()
		defer drainRecords(records)

		recordWriter, err := newRecordWriter(ctx, output, schema)
		if err != nil {
			logger.Log(ctx, ErrLevel(ctx, err), "Error creating new record writer", slog.Any("error", err))
			return
		}
		defer func() {
			logger = logger.With(
				slog.Int64("written", recordWriter.Written()),
			)
			logger.Debug("Closing record writer")
			if err := recordWriter.Close(); err != nil {
				logger.Log(ctx, ErrLevel(ctx, err), "Error closing record writer", slog.Any("error", err))
			}
		}()

		writeRecord := func(record arrow.RecordBatch) error {
			defer record.Release()
			return recordWriter.Write(record)
		}

		for record := range records {
			if err := writeRecord(record); err != nil {
				logger.Log(ctx, ErrLevel(ctx, err), "Error writing arrow record", slog.Any("error", err))
				return
			}
		}
	}(records)

	return records
}

func (h *arrowResultHandler) send(ctx context.Context, record arrow.RecordBatch) error {
	defer record.Release()

	for _, records := range h.recordChans {
		if err := h.sendTo(records, record); err != nil {
			return err
		}
	}
	return nil
}

func (h *arrowResultHandler) sendTo(records chan<- arrow.RecordBatch, record arrow.RecordBatch) error {
	record.Retain()

	for {
		select {
		case records <- record:
			return nil
		case <-h.ticker.C:
			// Send [api.StatusResult] messages, even if we're blocked waiting
			// to send an [arrow.RecordBatch] batch over the records channel.
			result := api.StatusResult{
				Status: api.StatusPending,
			}
			if err := h.writer.Flush(result); err != nil {
				record.Release()
				return err
			}
		}
	}
}

func (h *arrowResultHandler) Close() {
	if h.builder != nil {
		h.builder.Release()
	}
	for _, records := range h.recordChans {
		close(records)
	}
	h.wg.Wait()
}

func newRecordRowCount(ctx context.Context, record arrow.RecordBatch, oldRowCount int64) int64 {
	// Calculate the ideal number of rows per record based on the average
	// number of bytes per row in the last record and the target record size in
	// bytes.
	recordBytes := recordSizeBytes(record)
	bytesPerRow := (recordBytes / uint64(oldRowCount))
	newRowCount := int64(targetRecordBytes / bytesPerRow)

	// Clamp the new row count between the min and max. Also prevent very
	// large/sudden increases by limiting the increase to 2x the previous
	// row count (in case the last record was not a representative sample).
	newRowCount = min(newRowCount, oldRowCount*2, maxRecordRowCount)
	newRowCount = max(newRowCount, minRecordRowCount)

	if oldRowCount != newRowCount {
		logger := log.FromContext(ctx)
		logger.Debug("New record row count",
			slog.Int64("oldRowCount", oldRowCount),
			slog.Int64("newRowCount", newRowCount),
			slog.Uint64("recordBytes", recordBytes),
		)
	}
	return newRowCount
}

func recordSizeBytes(record arrow.RecordBatch) uint64 {
	var size uint64
	for _, array := range record.Columns() {
		size += array.Data().SizeInBytes()
	}
	return size
}

func drainChan[T any](c <-chan T) {
	for range c {
	}
}

func drainRecords(records <-chan arrow.RecordBatch) {
	for record := range records {
		record.Release()
	}
}
