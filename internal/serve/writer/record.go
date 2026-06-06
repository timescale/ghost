package writer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/arrio"
	"github.com/apache/arrow-go/v18/arrow/csv"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/timescale/ghost/internal/serve/api"
)

// newRecordWriter returns a type capable of writing a stream of [arrow.RecordBatch]
// batches to the format/destination requested in the [Output].
func newRecordWriter(ctx context.Context, output Output, schema *arrow.Schema) (*recordWriter, error) {
	writer, err := newDestinationWriter(ctx, output)
	if err != nil {
		return nil, err
	}

	// Remove __popsql_row_num__ field from schema if not requested.
	if !output.RowNum {
		metadata := schema.Metadata()
		schema = arrow.NewSchema(schema.Fields()[0:schema.NumFields()-1], &metadata)
	}

	switch output.Format {
	case api.OutputFormatArrowStream:
		return newArrowStreamRecordWriter(output, schema, writer)
	case api.OutputFormatArrowFile:
		return newArrowFileRecordWriter(output, schema, writer)
	case api.OutputFormatParquet:
		return newParquetRecordWriter(output, schema, writer)
	case api.OutputFormatCSV:
		return newCSVRecordWriter(output, schema, writer)
	case api.OutputFormatTSV:
		return newTSVRecordWriter(output, schema, writer)
	case api.OutputFormatJSON:
		return newJSONRecordWriter(output, schema, writer)
	default:
		return nil, fmt.Errorf("unexpected output format: %s", output.Format)
	}
}

func newArrowStreamRecordWriter(output Output, schema *arrow.Schema, writer io.WriteCloser) (*recordWriter, error) {
	opts := []ipc.Option{ipc.WithSchema(schema)}

	switch output.Compression {
	case api.OutputCompressionNone:
	case api.OutputCompressionGzip:
		// NOTE: gzip compression operates differently than the other forms of
		// compression, in that it compresses the entire output stream. This
		// exists for backwards-compatibility, but it's likely that using one
		// of the integration compression options produces better results.
		writer = newGzipWriter(writer)
	case api.OutputCompressionLZ4:
		opts = append(opts, ipc.WithLZ4())
	case api.OutputCompressionZstd:
		opts = append(opts, ipc.WithZstd())
	default:
		return nil, fmt.Errorf("unexpected compression type: %s", output.Compression)
	}

	ipcWriter := ipc.NewWriter(writer, opts...)
	return newRecordWriterFrom(output, schema, ipcWriter, writer), nil
}

func newArrowFileRecordWriter(output Output, schema *arrow.Schema, writer io.WriteCloser) (*recordWriter, error) {
	opts := []ipc.Option{ipc.WithSchema(schema)}

	switch output.Compression {
	case api.OutputCompressionNone:
	case api.OutputCompressionGzip:
		// NOTE: gzip compression operates differently than the other forms of
		// compression, in that it compresses the entire output stream. This
		// exists for backwards-compatibility, but it's likely that using one
		// of the integration compression options produces better results.
		writer = newGzipWriter(writer)
	case api.OutputCompressionLZ4:
		opts = append(opts, ipc.WithLZ4())
	case api.OutputCompressionZstd:
		opts = append(opts, ipc.WithZstd())
	default:
		return nil, fmt.Errorf("unexpected compression type: %s", output.Compression)
	}

	fileWriter, err := ipc.NewFileWriter(writer, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating Arrow file writer: %w", err)
	}
	return newRecordWriterFrom(output, schema, fileWriter, writer), nil
}

func newParquetRecordWriter(output Output, schema *arrow.Schema, writer io.WriteCloser) (*recordWriter, error) {
	var opts []parquet.WriterProperty
	switch output.Compression {
	case api.OutputCompressionNone:
		opts = append(opts, parquet.WithCompression(compress.Codecs.Uncompressed))
	case api.OutputCompressionBrotli:
		opts = append(opts, parquet.WithCompression(compress.Codecs.Brotli))
	case api.OutputCompressionGzip:
		opts = append(opts, parquet.WithCompression(compress.Codecs.Gzip))
	case api.OutputCompressionLZ4:
		opts = append(opts, parquet.WithCompression(compress.Codecs.Lz4Raw))
	case api.OutputCompressionSnappy:
		opts = append(opts, parquet.WithCompression(compress.Codecs.Snappy))
	case api.OutputCompressionZstd:
		opts = append(opts, parquet.WithCompression(compress.Codecs.Zstd))
	default:
		return nil, fmt.Errorf("unexpected compression type: %s", output.Compression)
	}

	fileWriter, err := pqarrow.NewFileWriter(
		schema,
		writer,
		parquet.NewWriterProperties(opts...),
		pqarrow.NewArrowWriterProperties(),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating Parquet file writer: %w", err)
	}
	parquetWriter := &parquetWriter{
		FileWriter: fileWriter,
	}
	return newRecordWriterFrom(output, schema, parquetWriter, writer), nil
}

// parquetWriter wraps a [pqarrow.FileWriter] so that the Write method uses
// WriteBuffered under the hood, which allows the writer to determine the ideal
// row group size, which results in considerable space savings.
type parquetWriter struct {
	*pqarrow.FileWriter
}

const maxParquetRowGroupBytes = 20 * 1024 * 1024 // 20 MiB

func (w *parquetWriter) Write(record arrow.RecordBatch) error {
	if err := w.FileWriter.WriteBuffered(record); err != nil {
		return err
	}
	written := w.FileWriter.RowGroupTotalBytesWritten()
	if written > maxParquetRowGroupBytes {
		w.FileWriter.NewBufferedRowGroup()
	}
	return nil
}

func newCSVRecordWriter(output Output, schema *arrow.Schema, writer io.WriteCloser) (*recordWriter, error) {
	switch output.Compression {
	case api.OutputCompressionNone:
	case api.OutputCompressionGzip:
		writer = newGzipWriter(writer)
	default:
		return nil, fmt.Errorf("unexpected compression type: %s", output.Compression)
	}

	csvWriter := csv.NewWriter(
		writer,
		schema,
		csv.WithHeader(true),
		csv.WithCRLF(true),
		csv.WithNullWriter(output.NullValue), // Defaults to "" for backwards-compatibility with existing clients
	)
	return newRecordWriterFrom(output, schema, csvWriter, writer), nil
}

func newTSVRecordWriter(output Output, schema *arrow.Schema, writer io.WriteCloser) (*recordWriter, error) {
	switch output.Compression {
	case api.OutputCompressionNone:
	case api.OutputCompressionGzip:
		writer = newGzipWriter(writer)
	default:
		return nil, fmt.Errorf("unexpected compression type: %s", output.Compression)
	}

	tsvWriter := csv.NewWriter(
		writer,
		schema,
		csv.WithHeader(true),
		csv.WithCRLF(true),
		csv.WithNullWriter(output.NullValue), // Defaults to "" for backwards-compatibility with existing clients
		csv.WithComma('\t'),
	)
	return newRecordWriterFrom(output, schema, tsvWriter, writer), nil
}

func newJSONRecordWriter(output Output, schema *arrow.Schema, writer io.WriteCloser) (*recordWriter, error) {
	switch output.Compression {
	case api.OutputCompressionNone:
	case api.OutputCompressionGzip:
		writer = newGzipWriter(writer)
	default:
		return nil, fmt.Errorf("unexpected compression type: %s", output.Compression)
	}

	jsonWriter := &jsonRecordWriter{w: writer}
	return newRecordWriterFrom(output, schema, jsonWriter, writer), nil
}

type jsonRecordWriter struct {
	w io.WriteCloser
}

func (w *jsonRecordWriter) Write(record arrow.RecordBatch) error {
	return array.RecordToJSON(record, w.w)
}

// flusher is implemented by some [io.WriteCloser] implementations (such as the
// gzipWriter). For these types, it is necessary to call Flush() in order to
// ensure all buffered bytes have actually been written to the underlying
// destination.
type flusher interface {
	Flush() error
}

type recordWriter struct {
	output       Output
	schema       *arrow.Schema
	recordWriter arrio.Writer
	writer       io.WriteCloser
	flusher      flusher
	written      int64
}

func newRecordWriterFrom(output Output, schema *arrow.Schema, rw arrio.Writer, w io.WriteCloser) *recordWriter {
	f, _ := w.(flusher)
	return &recordWriter{
		output:       output,
		schema:       schema,
		recordWriter: rw,
		writer:       w,
		flusher:      f,
	}
}

func (w *recordWriter) Write(record arrow.RecordBatch) error {
	// If there is a limit on the number of rows that should be written to this
	// output, truncate the final record to fit the limit, and stop writing
	// records after that.
	if w.output.Limit > 0 {
		remaining := w.output.Limit - w.written
		if remaining == 0 {
			return nil
		}
		if record.NumRows() > remaining {
			record = record.NewSlice(0, remaining)
			defer record.Release()
		}
	}
	w.written += record.NumRows()

	// Remove __popsql_row_num__ field from record if not requested.
	if !w.output.RowNum {
		record = array.NewRecordBatch(w.schema, record.Columns()[0:record.NumCols()-1], record.NumRows())
		defer record.Release()
	}

	if err := w.recordWriter.Write(record); err != nil {
		return err
	}

	// Flush records after writing them, if underlying writer implements the
	// [flusher] interface (e.g. as the gzipWriter does). This helps ensure
	// that complete records get sent to the client, rather than partial
	// records with the last bytes stuck in a buffer until the next record.
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}

func (w *recordWriter) Written() int64 {
	return w.written
}

// Not all [arrio.Writer] implementations close the underlying [io.WriteCloser]
// when their Close method is called, and some don't even have a Close method.
// This method exists to bridge that gap by closing both the [arrio.Writer] (if
// applicable), as well as the underlying [io.Writer].
func (w *recordWriter) Close() error {
	var err1 error
	if closer, ok := w.recordWriter.(io.Closer); ok {
		if err := closer.Close(); !w.ignoreCloseError(err) {
			err1 = err
		}
	}
	if err := w.writer.Close(); !w.ignoreCloseError(err) {
		return errors.Join(err1, err)
	}
	return err1
}

// These errors indicate that the writer has been closed. They are safe to
// ignore when trying to close the writer (since duplicate closes should be
// safe).
func (w *recordWriter) ignoreCloseError(err error) bool {
	return errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe)
}
