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
	"github.com/timescale/ghost/internal/common"
)

// Config configures a Server instance.
type Config struct {
	// Host is the bind address. Use "127.0.0.1" for loopback-only (recommended).
	Host string
	// Port is the bind port. 0 lets the OS choose a free port.
	Port int
	// App provides access to the ghost-api client and active project. Handlers
	// call App.Load on each request so OAuth tokens are refreshed and the user
	// can log in/out in another terminal without restarting the server.
	App *common.App
}

// Server wraps the HTTP server and exposes the resolved listen address.
type Server struct {
	cfg      Config
	srv      *http.Server
	ln       net.Listener
	addr     string
	runs     *runStore
	sessions *sessionStore
	state    *stateStore
}

// New constructs a Server with all routes registered. The listener is bound
// (so the resolved address is available immediately) but not yet serving.
// Call Serve to begin handling requests.
func New(cfg Config) (*Server, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.App == nil {
		return nil, errors.New("serve: app is required")
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	configDir := cfg.App.GetConfig().ConfigDir
	s := &Server{
		cfg:      cfg,
		ln:       ln,
		addr:     ln.Addr().String(),
		runs:     newRunStore(),
		sessions: newSessionStore(),
		state:    newStateStore(configDir),
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthzHandler())
	mux.Handle("GET /api/bootstrap", http.HandlerFunc(s.handleBootstrap))
	mux.Handle("GET /api/databases", http.HandlerFunc(s.handleDatabases))
	mux.Handle("POST /api/executeQuery", http.HandlerFunc(s.handleExecuteQuery))
	mux.Handle("POST /api/executeSessionQuery", http.HandlerFunc(s.handleExecuteSessionQuery))
	mux.Handle("POST /api/arrowResults", http.HandlerFunc(s.handleArrowResults))
	mux.Handle("POST /api/createSession", http.HandlerFunc(s.handleCreateSession))
	mux.Handle("POST /api/closeSession", http.HandlerFunc(s.handleCloseSession))
	mux.Handle("POST /api/sessionEvents", http.HandlerFunc(s.handleSessionEvents))
	mux.Handle("POST /api/cancelRun", http.HandlerFunc(s.handleCancelRun))
	mux.Handle("GET /api/state", http.HandlerFunc(s.handleGetState))
	mux.Handle("PUT /api/state", http.HandlerFunc(s.handlePutState))
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

// loadClient reloads credentials from disk (refreshing the OAuth token if
// needed) and returns a ghost-api client bound to the active project. Called
// per request so a long-running server doesn't keep using a stale token after
// it expires.
func (s *Server) loadClient(ctx context.Context) (api.ClientWithResponsesInterface, string, error) {
	_, client, projectID, err := s.cfg.App.Load(ctx)
	if err != nil {
		return nil, "", err
	}
	if client == nil {
		_, _, clientErr := s.cfg.App.GetClient()
		if clientErr != nil {
			return nil, "", clientErr
		}
		return nil, "", errors.New("authentication required")
	}
	return client, projectID, nil
}
