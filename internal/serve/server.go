package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/timescale/ghost/internal/api"
)

// Config configures a Server instance.
type Config struct {
	// Host is the bind address. Use "127.0.0.1" for loopback-only (recommended).
	Host string
	// Port is the bind port. 0 lets the OS choose a free port.
	Port int
	// Client is the authenticated ghost-api client.
	Client api.ClientWithResponsesInterface
	// ProjectID is the active space/project ID.
	ProjectID string
}

// Server wraps the HTTP server and exposes the resolved listen address.
type Server struct {
	cfg      Config
	srv      *http.Server
	ln       net.Listener
	addr     string
	runs     *runStore
	sessions *sessionStore
}

// New constructs a Server with all routes registered. The listener is bound
// (so the resolved address is available immediately) but not yet serving.
// Call Serve to begin handling requests.
func New(cfg Config) (*Server, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Client == nil {
		return nil, errors.New("serve: api client is required")
	}
	if cfg.ProjectID == "" {
		return nil, errors.New("serve: project id is required")
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	s := &Server{
		cfg:      cfg,
		ln:       ln,
		addr:     ln.Addr().String(),
		runs:     newRunStore(),
		sessions: newSessionStore(),
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthzHandler())
	mux.Handle("GET /api/bootstrap", newBootstrapHandler(cfg.ProjectID))
	mux.Handle("GET /api/databases", newDatabasesHandler(cfg.Client, cfg.ProjectID))
	mux.Handle("POST /api/executeQuery", http.HandlerFunc(s.handleExecuteQuery))
	mux.Handle("POST /api/executeSessionQuery", http.HandlerFunc(s.handleExecuteSessionQuery))
	mux.Handle("POST /api/arrowResults", http.HandlerFunc(s.handleArrowResults))
	mux.Handle("POST /api/createSession", http.HandlerFunc(s.handleCreateSession))
	mux.Handle("POST /api/closeSession", http.HandlerFunc(s.handleCloseSession))
	mux.Handle("POST /api/sessionEvents", http.HandlerFunc(s.handleSessionEvents))
	mux.Handle("POST /api/cancelRun", http.HandlerFunc(s.handleCancelRun))
	mux.Handle("/", newAssetHandler())

	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return s, nil
}

// Addr returns the resolved listen address (with the OS-chosen port if Port
// was 0).
func (s *Server) Addr() string { return s.addr }

// URL returns the http://addr URL clients should connect to.
func (s *Server) URL() string { return "http://" + s.addr }

// Serve starts handling requests and blocks until ctx is canceled. On
// cancellation the server is gracefully shut down with a 5s deadline.
func (s *Server) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := s.srv.Serve(s.ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
		s.sessions.closeAll()
		return <-errCh
	case err := <-errCh:
		s.sessions.closeAll()
		return err
	}
}

func healthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
}
