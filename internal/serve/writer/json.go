package writer

import (
	"encoding/json"
	"net/http"

	"github.com/timescale/ghost/internal/serve/api"
)

// jsonWriter is a type capable of writing a stream of gzipped
// newline-delimited JSON to an [http.ResponseWriter].
type jsonWriter struct {
	*gzipWriter
	encoder *json.Encoder
}

func newJSONWriter(w http.ResponseWriter) *jsonWriter {
	g := newGzipWriter(w)
	return &jsonWriter{
		gzipWriter: g,
		encoder:    json.NewEncoder(g),
	}
}

func (w *jsonWriter) Write(v any) error {
	if err := w.encoder.Encode(v); err != nil {
		return &WriteError{
			Msg: "error encoding value to JSON",
			Err: err,
		}
	}
	return nil
}

func (w *jsonWriter) Flush(v any) error {
	if err := w.Write(v); err != nil {
		return err
	}
	return w.gzipWriter.Flush()
}

func (w *jsonWriter) WriteError(err error) error {
	// TODO: This should technically be an ErrorResult, with all of the
	// required fields.
	if err := w.encoder.Encode(api.NewErrorResponse(err)); err != nil {
		return &WriteError{
			Msg: "error encoding error to JSON",
			Err: err,
		}
	}
	return nil
}
