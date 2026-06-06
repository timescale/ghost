package serve

import (
	"compress/gzip"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/timescale/ghost/internal/serve/api"
)

type Router struct {
	*httprouter.Router
	middleware []Middleware
}

func NewRouter(middleware ...Middleware) *Router {
	return &Router{
		Router: httprouter.New(),

		middleware: middleware,
	}
}

type Middleware func(handler http.Handler) http.Handler

func (r *Router) GET(path string, handler http.HandlerFunc, middleware ...Middleware) {
	r.Handler(http.MethodGet, path, r.wrap(handler, middleware...))
}

func (r *Router) POST(path string, handler http.HandlerFunc, middleware ...Middleware) {
	r.Handler(http.MethodPost, path, r.wrap(handler, middleware...))
}

func (r *Router) PUT(path string, handler http.HandlerFunc, middleware ...Middleware) {
	r.Handler(http.MethodPut, path, r.wrap(handler, middleware...))
}

func (r *Router) PATCH(path string, handler http.HandlerFunc, middleware ...Middleware) {
	r.Handler(http.MethodPatch, path, r.wrap(handler, middleware...))
}

func (r *Router) DELETE(path string, handler http.HandlerFunc, middleware ...Middleware) {
	r.Handler(http.MethodDelete, path, r.wrap(handler, middleware...))
}

func (r *Router) NotFound(handler http.HandlerFunc, middleware ...Middleware) {
	r.Router.NotFound = r.wrap(handler, middleware...)
}

func (r *Router) MethodNotAllowed(handler http.HandlerFunc, middleware ...Middleware) {
	r.Router.MethodNotAllowed = r.wrap(handler, middleware...)
}

func (r *Router) wrap(handler http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	for i := len(r.middleware) - 1; i >= 0; i-- {
		handler = r.middleware[i](handler)
	}
	return handler
}

func internalServerError(w http.ResponseWriter, logger *slog.Logger) {
	writeError(w, http.StatusInternalServerError, api.ErrInternalServer, logger)
}

func writeError(w http.ResponseWriter, statusCode int, err error, logger *slog.Logger) {
	body := api.NewErrorResponse(err)
	writeJSON(w, statusCode, body, logger)
}

func writeNormalizedError(w http.ResponseWriter, statusCode int, err *api.NormalizedError, logger *slog.Logger) {
	body := api.NewNormalizedErrorResponse(err)
	writeJSON(w, statusCode, body, logger)
}

func writeJSON(w http.ResponseWriter, statusCode int, body any, logger *slog.Logger) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		response := "Internal Server Error"
		logger.Error("Error marshalling response body to json", slog.Any("error", err))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writeResponse(w, http.StatusInternalServerError, []byte(response), logger)
	} else {
		w.Header().Set("Content-Type", "application/json")
		writeResponse(w, statusCode, jsonBody, logger)
	}
}

func writeResponse(w http.ResponseWriter, statusCode int, body []byte, logger *slog.Logger) {
	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(statusCode)

	gzipWriter := gzip.NewWriter(w)
	if _, err := gzipWriter.Write(body); err != nil {
		logger.Error("Error writing response body", slog.Any("error", err))
	}
	if err := gzipWriter.Close(); err != nil {
		logger.Error("Error closing gzip writer", slog.Any("error", err))
	}
}
