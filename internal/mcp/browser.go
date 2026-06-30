package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/ghost/internal/common"
	"github.com/timescale/ghost/internal/serve"
)

// decodeImageDataURL parses a data URL (e.g. "data:image/png;base64,iVBOR...")
// into an MCP [mcp.ImageContent]. Returns nil if the string is empty or not a
// base64 data URL.
func decodeImageDataURL(dataURL string) (*mcp.ImageContent, error) {
	if dataURL == "" {
		return nil, nil
	}
	const prefix = "data:"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, errors.New("chart image is not a data URL")
	}
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return nil, errors.New("malformed image data URL")
	}
	meta := dataURL[len(prefix):comma]
	// The metadata is the MIME type followed by optional ";"-separated
	// parameters, with ";base64" (when present) as the final token. Inspect the
	// last token rather than assuming base64 is the second one, so URLs carrying
	// extra parameters (e.g. "data:image/png;charset=utf-8;base64,...") aren't
	// wrongly rejected.
	if !strings.HasSuffix(meta, ";base64") {
		return nil, errors.New("image data URL is not base64-encoded")
	}
	mimeType := strings.TrimSuffix(meta, ";base64")
	data, err := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode image data: %w", err)
	}
	return &mcp.ImageContent{Data: data, MIMEType: mimeType}, nil
}

// browserController lazily starts an in-process `ghost serve` web server and
// drives it via the agent [serve.Bridge]. It is owned by the MCP [Server] and
// only used by the local (stdio) transport — opening a browser from a remote
// HTTP server is meaningless, so the visualize/chart/ui_state tools are gated
// to local mode.
//
// The server is started on first use (first visualize/chart/ui_state tool
// call), the browser is opened when no client is connected, and everything is
// torn down when the MCP server shuts down.
type browserController struct {
	app    *common.App
	logger *slog.Logger

	mu        sync.Mutex
	bridge    *serve.Bridge
	store     *serve.Store
	server    *serve.Server
	stopWatch chan struct{}
}

func newBrowserController(app *common.App, logger *slog.Logger) *browserController {
	return &browserController{
		app:    app,
		logger: ensureLogger(logger),
	}
}

// ensureStarted lazily starts the in-process serve server (bound to an
// ephemeral loopback port) and returns the bridge. Subsequent calls return the
// already-running instance.
func (c *browserController) ensureStarted(ctx context.Context) (*serve.Bridge, error) {
	// Verify the API client is available before starting the server or opening
	// the browser. Without this, a logged-out user gets an opaque "no browser
	// connected" timeout: the web app fails /api/bootstrap and never connects an
	// active client, instead of the real auth/config error surfacing here. This
	// runs on every call (before the already-started early-return) so expired
	// credentials are caught too.
	if _, _, err := c.app.GetClient(); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bridge != nil {
		return c.bridge, nil
	}

	bridge := serve.NewBridge()
	configDir := c.app.GetConfig().ConfigDir
	store := serve.NewStore(configDir, c.logger)
	handler := serve.NewHandler(serve.HandlerConfig{
		App:    c.app,
		Store:  store,
		Logger: c.logger,
		Bridge: bridge,
	})
	server := serve.NewServer("localhost", 0, handler.Handler(), c.logger)
	if err := server.Start(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to start web server: %w", err)
	}

	// Drain the server's error channel. The send in [serve.Server.Start]'s
	// goroutine is unbuffered, so without a reader a real serving error would
	// block that goroutine forever (a leak) and the failure would never surface
	// here — subsequent tool calls would just time out with an opaque "no browser
	// connected" instead of the real cause. The watcher exits on the first error
	// (the goroutine sends at most one) or when [browserController.Close] closes
	// stopWatch on clean shutdown (where Serve returns http.ErrServerClosed and
	// never sends).
	errs := server.Errors()
	stop := make(chan struct{})
	go func() {
		select {
		case err := <-errs:
			if err != nil {
				c.logger.Error("web UI server stopped unexpectedly", slog.Any("error", err))
			}
		case <-stop:
		}
	}()

	c.bridge = bridge
	c.store = store
	c.server = server
	c.stopWatch = stop
	c.logger.Info("Started in-process web UI for agent visualization", slog.String("url", server.URL()))
	return bridge, nil
}

// url returns the URL of the running server, or "" if it hasn't been started.
func (c *browserController) url() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.server == nil {
		return ""
	}
	return c.server.URL()
}

// ensureClient starts the server (if needed), opens a browser when no client is
// connected, and waits for an active client to be ready to receive commands.
func (c *browserController) ensureClient(ctx context.Context) (*serve.Bridge, error) {
	bridge, err := c.ensureStarted(ctx)
	if err != nil {
		return nil, err
	}

	if !bridge.HasActiveClient() {
		url := c.url()
		c.logger.Info("Opening browser for agent visualization", slog.String("url", url))
		if err := common.OpenBrowser(url); err != nil {
			c.logger.Warn("Failed to open browser; open it manually",
				slog.String("url", url),
				slog.Any("error", err),
			)
		}
		if err := bridge.WaitForActiveClient(ctx); err != nil {
			if errors.Is(err, serve.ErrNoActiveClient) {
				return nil, fmt.Errorf("no browser connected to %s; open it to enable visualization", url)
			}
			return nil, err
		}
	}

	return bridge, nil
}

// request dispatches a command to the active browser client and unmarshals the
// JSON response into out (which may be nil to ignore the response body).
func (c *browserController) request(ctx context.Context, commandType browserCommand, payload any, out any) error {
	bridge, err := c.ensureClient(ctx)
	if err != nil {
		return err
	}

	data, err := bridge.Request(ctx, string(commandType), payload)
	if err != nil {
		return err
	}
	if out != nil && len(data) > 0 {
		// Decode numbers as json.Number, not float64, so cell values keep their
		// exact literal text. Plain float64 decoding would re-render large or
		// whole numbers in exponent form (e.g. 10000000 -> "1e+07") when
		// stringified, diverging from the server-side query path's text output.
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(out); err != nil {
			return fmt.Errorf("failed to parse browser response: %w", err)
		}
	}
	return nil
}

// Close tears down the running server and store. Safe to call when nothing was
// started.
func (c *browserController) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.server == nil {
		return nil
	}
	// Stop the error watcher before closing the server: Close makes Serve return
	// http.ErrServerClosed (no send on the error channel), so the watcher would
	// otherwise block forever.
	close(c.stopWatch)
	err := c.server.Close()
	// Store.Close() returns no error — it logs any session-teardown failures
	// internally.
	c.store.Close()
	c.server = nil
	c.store = nil
	c.bridge = nil
	c.stopWatch = nil
	return err
}
