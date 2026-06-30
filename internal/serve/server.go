package serve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Server is a wrapper around [http.Server] that encapsulates some standard
// configuration and boilerplate for starting and stopping the server.
type Server struct {
	host    string
	port    int
	handler http.Handler
	logger  *slog.Logger

	httpServer *http.Server
	errChan    chan error
}

// NewServer creates a new instance of [Server]. The host may be empty to bind
// all interfaces, though callers should generally pass a loopback address
// (e.g. "localhost"). To start the server, call [Server.Start].
func NewServer(host string, port int, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		host:    host,
		port:    port,
		handler: handler,
		logger:  logger,
	}
}

// Start starts listening for incoming HTTP requests on the configured host and
// port. If the port is 0, an ephemeral port is chosen, and the stored port is
// subsequently updated to the chosen port (useful in automated tests). Note
// that the context argument only controls the cancellation of this function
// itself - once started, the server can only be stopped by calling
// [Server.Close].
func (s *Server) Start(ctx context.Context) error {
	logger := s.logger

	if s.httpServer != nil || s.errChan != nil {
		return errors.New("server has already been started")
	}

	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	var netConfig net.ListenConfig
	listener, err := netConfig.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	// Update port if using an ephemeral port (i.e. if original port was 0).
	s.port = listener.Addr().(*net.TCPAddr).Port

	stdLog := slog.NewLogLogger(logger.Handler(), slog.LevelError)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           s.handler,
		ErrorLog:          stdLog,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errChan := make(chan error)

	s.httpServer = httpServer
	s.errChan = errChan

	go func() {
		// NOTE: It's important that we don't use s.httpServer/s.errChan here,
		// because that could lead to a data race with Close setting them to
		// nil. Instead, use local variable closures to reference them.
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	return nil
}

// URL returns the http URL clients should connect to (with the OS-chosen port
// if the configured port was 0). Only accurate after [Server.Start] has been
// called.
func (s *Server) URL() string {
	u := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(s.host, strconv.Itoa(s.port)),
	}
	return u.String()
}

// Errors returns a channel on which server errors are returned. Returns a nil
// channel if the server is not running (e.g. [Server.Start] has not yet been
// called or if [Server.Close] has already been called).
func (s *Server) Errors() <-chan error {
	return s.errChan
}

// Close immediately shuts down the HTTP server started by [Server.Start],
// closing all active connections without waiting for in-flight requests to
// complete. If [Server.Start] has not been called successfully, or if the HTTP
// server has already been closed, Close is a no-op. Note that it is not safe to
// call this method concurrently with itself or [Server.Start].
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}

	if err := s.httpServer.Close(); err != nil {
		return fmt.Errorf("error closing server: %w", err)
	}
	s.httpServer = nil
	s.errChan = nil

	return nil
}
