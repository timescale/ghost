package common

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"github.com/timescale/ghost/internal/analytics"
	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/config"
	"github.com/timescale/ghost/internal/util"
)

// ghostClientID must match the constant in ghost-api
const ghostClientID = "3a588b0a-db13-4ae3-a5b1-8826e6a59b8a"

// errBrowserOpenFailed is returned by getTokenViaCodeGrantFlow when the
// browser cannot be opened, signaling that we should fall back to the device flow.
var errBrowserOpenFailed = errors.New("failed to open browser")

type oauthLogin struct {
	authURL        string // Ghost API URL for /oauth/github
	tokenURL       string // Ghost API URL for /oauth/token
	deviceCodeURL  string // Ghost API URL for /oauth/github/device/code
	deviceTokenURL string // Ghost API URL for /oauth/github/device/token
	successURL     string // Ghost API URL for /oauth/success
	headless       bool
	out            io.Writer
}

// LoginResult holds user information returned by a successful login.
type LoginResult struct {
	Name    string
	Email   string
	SpaceID string
}

// Login performs the full OAuth login flow: authenticates the user via GitHub
// OAuth, validates the session, and stores credentials securely.
// If headless is true, uses the device authorization flow instead of opening a browser.
// Progress messages are written to out (pass nil to suppress output).
func Login(ctx context.Context, app *App, headless bool, out io.Writer) (*LoginResult, error) {
	cfg := app.GetConfig()

	l := &oauthLogin{
		authURL:        cfg.APIURL + "/oauth/github",
		tokenURL:       cfg.APIURL + "/oauth/token",
		deviceCodeURL:  cfg.APIURL + "/oauth/github/device/code",
		deviceTokenURL: cfg.APIURL + "/oauth/github/device/token",
		successURL:     cfg.APIURL + "/oauth/success",
		headless:       headless,
		out:            ensureOut(out),
	}

	// Authenticate via OAuth to get tokens
	token, err := l.loginWithOAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Create an API client using the OAuth token (freshly minted, no refresh needed)
	client, err := api.NewGhostClient(cfg.APIURL, api.TokenAuth(token))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Get the user's existing Ghost space, or create a new one
	spaceID, err := l.findOrCreateSpace(ctx, client)
	if err != nil {
		if apiErr, ok := errors.AsType[*api.Error](err); ok && util.Deref(apiErr.Code) == api.ErrorCodeSignupsDisabled {
			return nil, apiErr
		}
		return nil, fmt.Errorf("failed to select space: %w", err)
	}

	// Identify the user for analytics
	authInfo, err := identifyWithUserToken(ctx, cfg, client, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to identify user: %w", err)
	}

	// Store the OAuth token and space ID securely
	if err := cfg.StoreCredentials(config.Credentials{
		Token:   token,
		SpaceID: spaceID,
	}); err != nil {
		return nil, fmt.Errorf("failed to store credentials: %w", err)
	}

	// Store the client on the App so that the analytics wrapper
	// can record this event.
	app.SetClient(client, spaceID)

	result := &LoginResult{
		Name:    authInfo.User.Name,
		Email:   authInfo.User.Email,
		SpaceID: spaceID,
	}

	return result, nil
}

func ensureOut(out io.Writer) io.Writer {
	if out != nil {
		return out
	}
	return io.Discard
}

func (l *oauthLogin) loginWithOAuth(ctx context.Context) (*oauth2.Token, error) {
	if l.headless {
		return l.getTokenViaDeviceFlow(ctx)
	}
	token, err := l.getTokenViaCodeGrantFlow(ctx)
	if errors.Is(err, errBrowserOpenFailed) {
		fmt.Fprintln(l.out, "Failed to open browser. Falling back to device authorization flow...")
		return l.getTokenViaDeviceFlow(ctx)
	}
	return token, err
}

func (l *oauthLogin) getTokenViaCodeGrantFlow(ctx context.Context) (*oauth2.Token, error) {
	// Generate PKCE parameters
	codeVerifier := oauth2.GenerateVerifier()

	// Generate random state string (to guard against CSRF attacks)
	state, err := generateRandomState(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random state: %w", err)
	}

	// Start local HTTP server for handling the OAuth callback
	server, err := l.startOAuthServer(state, codeVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to create local server: %w", err)
	}
	defer func() {
		if err := server.server.Shutdown(ctx); err != nil {
			fmt.Fprintf(l.out, "Failed to close local server: %s\n", err)
		}
	}()

	// Open browser
	authURL := server.oauthCfg.AuthCodeURL(state, oauth2.S256ChallengeOption(codeVerifier))
	fmt.Fprintln(l.out, "Opening browser for authentication...")
	errCh := OpenBrowserAsync(authURL)

	// Wait for callback with timeout
	timeout := time.After(5 * time.Minute)
	for {
		select {
		case <-errCh:
			return nil, errBrowserOpenFailed
		case result := <-server.resultChan:
			return result.token, result.err
		case <-timeout:
			return nil, errors.New("authorization timeout - no callback received within 5 minutes")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func generateRandomState(length int) (string, error) {
	data := make([]byte, length)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(data)[:length], nil
}

type oauthServer struct {
	server     *http.Server
	oauthCfg   oauth2.Config
	resultChan <-chan oauthResult
}

type oauthResult struct {
	token *oauth2.Token
	err   error
}

func (l *oauthLogin) startOAuthServer(expectedState, codeVerifier string) (*oauthServer, error) {
	// Start listening on an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen on local port: %w", err)
	}

	// Build OAuth config with localhost redirect URI
	port := listener.Addr().(*net.TCPAddr).Port
	oauthCfg := oauth2.Config{
		ClientID: ghostClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:   l.authURL,
			TokenURL:  l.tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: fmt.Sprintf("http://localhost:%d/callback", port),
	}

	// Start local HTTP server for callback
	resultChan := make(chan oauthResult, 1)
	mux := http.NewServeMux()
	mux.Handle("GET /callback", &oauthCallback{
		oauthCfg:      oauthCfg,
		expectedState: expectedState,
		codeVerifier:  codeVerifier,
		successURL:    l.successURL,
		resultChan:    resultChan,
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			resultChan <- oauthResult{
				err: fmt.Errorf("failed to serve requests: %w", err),
			}
		}
	}()

	return &oauthServer{
		server:     server,
		oauthCfg:   oauthCfg,
		resultChan: resultChan,
	}, nil
}

type oauthCallback struct {
	oauthCfg      oauth2.Config
	expectedState string
	codeVerifier  string
	successURL    string // Ghost API URL for /oauth/success
	resultChan    chan<- oauthResult
}

func (c *oauthCallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Validate state parameter
	state := query.Get("state")
	if state != c.expectedState {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid state parameter")
		c.sendError(errors.New("invalid state parameter"))
		return
	}

	// Get authorization code
	code := query.Get("code")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Missing authorization code")
		c.sendError(errors.New("missing authorization code in callback"))
		return
	}

	// Exchange authorization code for tokens
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, api.HTTPClient)
	token, err := c.oauthCfg.Exchange(ctx, code, oauth2.VerifierOption(c.codeVerifier))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to exchange authorization code for tokens")
		c.sendError(fmt.Errorf("failed to exchange code for tokens: %w", err))
		return
	}

	// Redirect to the API-hosted success page
	http.Redirect(w, r, c.successURL, http.StatusFound)

	c.resultChan <- oauthResult{
		token: token,
	}
}

func (c *oauthCallback) sendError(err error) {
	c.resultChan <- oauthResult{err: err}
}

func (l *oauthLogin) getTokenViaDeviceFlow(ctx context.Context) (*oauth2.Token, error) {
	cfg := oauth2.Config{
		ClientID: ghostClientID,
		Endpoint: oauth2.Endpoint{
			DeviceAuthURL: l.deviceCodeURL,
			TokenURL:      l.deviceTokenURL,
			AuthStyle:     oauth2.AuthStyleInParams,
		},
	}

	// Use the app's HTTP client for all oauth2 requests
	ctx = context.WithValue(ctx, oauth2.HTTPClient, api.HTTPClient)

	// Request device code from gateway
	deviceAuth, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate device flow: %w", err)
	}

	// Display instructions to user
	fmt.Fprintf(l.out, "\nTo authenticate, visit: %s\n", deviceAuth.VerificationURI)
	fmt.Fprintf(l.out, "and enter code: %s\n\n", deviceAuth.UserCode)
	fmt.Fprintln(l.out, "Waiting for authorization (this can take a few seconds after you enter the code)...")

	// Best effort attempt to open browser
	OpenBrowserAsync(deviceAuth.VerificationURI)

	// Poll for authorization (blocks until user authorizes or code expires)
	token, err := cfg.DeviceAccessToken(ctx, deviceAuth)
	if err != nil {
		return nil, fmt.Errorf("authorization failed: %w", err)
	}

	return token, nil
}

// findOrCreateSpace selects the Ghost space to use after login. Every Ghost
// user should own a personal space, so if the user doesn't own one yet (e.g. a
// new user whose account was created by accepting an invitation to someone
// else's space), one is created for them. The login defaults to the owned
// space; any additional spaces the user has only joined are listed with a hint
// for switching to them.
func (l *oauthLogin) findOrCreateSpace(ctx context.Context, client api.ClientWithResponsesInterface) (string, error) {
	// First, list the user's existing spaces
	listResp, err := client.ListSpacesWithResponse(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list spaces: %w", err)
	}
	if listResp.StatusCode() != 200 {
		if listResp.JSONDefault != nil {
			return "", fmt.Errorf("failed to list spaces: %w", listResp.JSONDefault)
		}
		return "", fmt.Errorf("failed to list spaces: unexpected status %d", listResp.StatusCode())
	}
	if listResp.JSON200 == nil {
		return "", errors.New("failed to list spaces: empty response from API")
	}

	// Partition into the space the user owns and the ones they've only joined
	// (e.g. via invitation). Ownership is the API "owner" role.
	var ownedSpace *api.Space
	var joined []api.Space
	for _, space := range *listResp.JSON200 {
		if space.Role != nil && *space.Role == api.MemberRoleOwner {
			ownedSpace = &space
		} else {
			joined = append(joined, space)
		}
	}

	// If the user doesn't own a space yet, create one. The name is omitted so
	// the API applies the owner-derived default name (e.g. "Jane Doe's space").
	if ownedSpace == nil {
		createResp, err := client.CreateSpaceWithResponse(ctx, api.CreateSpaceJSONRequestBody{})
		if err != nil {
			return "", fmt.Errorf("failed to create space: %w", err)
		}
		if createResp.StatusCode() != 201 {
			if createResp.JSONDefault != nil {
				return "", fmt.Errorf("failed to create space: %w", createResp.JSONDefault)
			}
			return "", fmt.Errorf("failed to create space: unexpected status %d", createResp.StatusCode())
		}
		if createResp.JSON201 == nil {
			return "", errors.New("failed to create space: empty response from API")
		}
		fmt.Fprintf(l.out, "Created space: %s\n", createResp.JSON201.ID)
		ownedSpace = createResp.JSON201
	} else {
		fmt.Fprintf(l.out, "Found space: %s\n", ownedSpace.ID)
	}

	// Default to the owned space. If the user also belongs to other spaces,
	// point them at `ghost space use` for switching.
	if len(joined) > 0 {
		fmt.Fprintln(l.out, "\nYou also belong to:")
		for _, space := range joined {
			fmt.Fprintf(l.out, "  %s (%s)\n", space.Name, space.ID)
		}
		fmt.Fprintln(l.out, "Run 'ghost space use <id>' to switch the current space.")
	}

	return ownedSpace.ID, nil
}

// identifyWithUserToken calls the /auth/info endpoint to validate a user token
// and identify the user for analytics. The spaceID must be provided since user
// token auth info does not include it.
func identifyWithUserToken(ctx context.Context, cfg *config.Config, client api.ClientWithResponsesInterface, spaceID string) (*api.AuthInfo, error) {
	// Call the /auth/info endpoint to validate credentials and get auth info
	resp, err := client.AuthInfoWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	// Check the response status
	if resp.StatusCode() != 200 {
		if resp.JSONDefault != nil {
			return nil, resp.JSONDefault
		}
		return nil, fmt.Errorf("unexpected API response: %d", resp.StatusCode())
	}

	if resp.JSON200 == nil {
		return nil, errors.New("empty response from API")
	}

	authInfo := resp.JSON200

	// Identify the user with analytics
	a := analytics.New(cfg, client, spaceID)
	a.Identify(
		analytics.Property("userId", authInfo.User.ID),
		analytics.Property("email", authInfo.User.Email),
	)

	return authInfo, nil
}
