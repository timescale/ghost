package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/timescale/ghost/internal/log"
	"github.com/timescale/ghost/internal/serve/api"
)

func logRequests(logger *slog.Logger) Middleware {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Use the client-provided request ID, or generate one.
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Create a request-scoped logger.
			ctx, logger := log.NewContext(ctx, logger.With(
				slog.String("method", r.Method),
				slog.String("from", r.RemoteAddr),
				slog.String("url", r.URL.String()),
				slog.String("requestId", requestID),
			))

			// Log the request duration.
			defer func(start time.Time) {
				logger.Debug("Request handled",
					slog.Duration("elapsed", time.Since(start)),
				)
			}(time.Now())

			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func handlePanics() Middleware {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := log.FromContext(ctx)

			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					logger.Error(fmt.Sprintf("Request handler panic: %s\n%s", r, stack))
					writeError(w, http.StatusInternalServerError, api.ErrInternalServer, logger)
				}
			}()

			handler.ServeHTTP(w, r)
		})
	}
}

func contentTypeJSON() Middleware {
	return contentType("application/json")
}

func contentType(required string) Middleware {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := log.FromContext(ctx)

			header := r.Header["Content-Type"]
			if len(header) != 1 || header[0] != required {
				logger.Warn("Invalid content type",
					slog.Any("header", header),
				)
				writeError(w, http.StatusBadRequest, &InvalidContentTypeError{
					Required: required,
				}, logger)
				return
			}

			handler.ServeHTTP(w, r)
		})
	}
}

type requestKey struct{}

func requestFromContext(ctx context.Context) any {
	key := requestKey{}
	req := ctx.Value(key)
	if req == nil {
		panic(errors.New("unmarshalRequestBody middleware not configured"))
	}
	return req
}

func unmarshalRequest[T any]() Middleware {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := log.FromContext(ctx)

			body, err := io.ReadAll(r.Body)
			if err != nil {
				logger.Warn("Error reading request body", slog.Any("error", err))
				writeError(w, http.StatusBadRequest, &InvalidJSONBodyError{}, logger)
				return
			}

			var req T
			if err := json.Unmarshal(body, &req); err != nil {
				logger.Warn("Error unmarshalling request body", slog.Any("error", err))
				writeError(w, http.StatusBadRequest, unmarshalError(err), logger)
				return
			}

			key := requestKey{}
			ctx = context.WithValue(ctx, key, &req)

			ctx, _ = log.NewContext(ctx, logger.With(
				slog.Any("request", req),
			))

			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func unmarshalError(err error) error {
	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		return &InvalidJSONBodyError{
			Err: syntaxError,
		}
	}

	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		return &InvalidJSONBodyError{
			Err: typeError,
		}
	}

	return &InvalidJSONBodyError{}
}

type validator interface {
	Validate() error
}

func validateRequest() Middleware {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := log.FromContext(ctx)
			req, ok := requestFromContext(ctx).(validator)
			if !ok {
				panic(errors.New("request type does not implement validator interface"))
			}

			if err := req.Validate(); err != nil {
				logger.Warn("Invalid request", slog.Any("error", err))
				writeError(w, http.StatusBadRequest, err, logger)
				return
			}

			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
