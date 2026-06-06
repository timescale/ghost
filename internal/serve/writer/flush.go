package writer

import "net/http"

// FlushWriter wraps the Write method of an [http.ResponseWriter] so that all
// calls to write are followed by a call to Flush. The [http.ResponseWriter]
// must implement [http.Flusher].
type FlushWriter struct {
	rw      http.ResponseWriter
	flusher http.Flusher
}

func NewFlushWriter(rw http.ResponseWriter) *FlushWriter {
	return &FlushWriter{
		rw:      rw,
		flusher: rw.(http.Flusher),
	}
}

func (w *FlushWriter) Write(b []byte) (int, error) {
	n, err := w.rw.Write(b)
	w.flusher.Flush()
	return n, err
}
