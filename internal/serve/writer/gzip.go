package writer

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
)

// gzipWriter wraps an [io.Writer] with a [gzip.Writer] and provides some
// additional functionality related to flushing and closing the underlying
// [io.Writer] when the [gzip.Writer] is flushed or closed.
type gzipWriter struct {
	*gzip.Writer
	writer  io.Writer
	flusher http.Flusher
}

func newGzipWriter(w io.Writer) *gzipWriter {
	flusher, _ := w.(http.Flusher)
	return &gzipWriter{
		Writer:  gzip.NewWriter(w),
		writer:  w,
		flusher: flusher,
	}
}

// Flush the internal buffer of the [gzip.Writer]. If the underlying
// [io.Writer] implements [http.Flusher], call its Flush method as well.
func (g *gzipWriter) Flush() error {
	if err := g.Writer.Flush(); err != nil {
		return &WriteError{
			Msg: "error flushing gzip writer",
			Err: err,
		}
	}
	if g.flusher != nil {
		g.flusher.Flush()
	}
	return nil
}

// Close the [gzip.Writer]. If the underlying [io.Writer] implements
// [io.Closer], call its Close method as well (this behavior differs from that
// of a plan [gzip.Writer]).
func (g *gzipWriter) Close() error {
	err := g.Writer.Close()
	if closer, ok := g.writer.(io.Closer); ok {
		return errors.Join(err, closer.Close())
	}
	return err
}
