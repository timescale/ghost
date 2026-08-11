package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/api/mock"
)

// startFakeTokenServer starts an httptest.Server that handles the OAuth token
// exchange endpoint (POST /oauth/token), the success redirect page
// (GET /oauth/success), and the device authorization flow endpoints
// (POST /oauth/github/device/code, POST /oauth/github/device/token).
// The server is automatically closed when the test ends.
func startFakeTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if r.FormValue("code") == "" || r.FormValue("code_verifier") == "" {
			http.Error(w, "missing params", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "mock-access-token",
			"refresh_token": "mock-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})
	mux.HandleFunc("GET /oauth/success", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /oauth/github/device/code", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if r.FormValue("client_id") == "" {
			http.Error(w, "missing client_id", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "test-device-code",
			"user_code":        "TEST-1234",
			"verification_uri": "https://github.com/login/device",
			"interval":         1,
			"expires_in":       900,
		})
	})
	mux.HandleFunc("POST /oauth/github/device/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if r.FormValue("device_code") == "" {
			http.Error(w, "missing device_code", http.StatusBadRequest)
			return
		}
		if r.FormValue("client_id") == "" {
			http.Error(w, "missing client_id", http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			http.Error(w, "invalid grant_type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "mock-device-token",
			"refresh_token": "mock-device-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// mockBrowserForOAuth returns an OpenBrowser replacement that simulates the
// OAuth callback. It parses the auth URL to extract redirect_uri and state,
// then makes an HTTP GET to the callback URL with a fake auth code. This runs
// synchronously — OpenBrowserAsync already calls OpenBrowser in a goroutine,
// so the callback happens concurrently with the select loop that waits for it.
func mockBrowserForOAuth() func(string) error {
	return func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		redirectURI := parsed.Query().Get("redirect_uri")
		state := parsed.Query().Get("state")
		if redirectURI == "" || state == "" {
			return errors.New("missing redirect_uri or state in auth URL")
		}
		callbackURL := fmt.Sprintf("%s?code=test-auth-code&state=%s", redirectURI, state)
		resp, err := http.Get(callbackURL)
		if err != nil {
			return fmt.Errorf("callback request failed: %w", err)
		}
		resp.Body.Close()
		return nil
	}
}

func TestLoginCmd(t *testing.T) {
	tokenServer := startFakeTokenServer(t)

	t.Run("success with existing space", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		spaces := []api.Space{{ID: "space-123", Name: "My Space", Role: new(api.MemberRoleOwner)}}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &spaces,
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoTypeUser,
					User: &api.UserInfo{Name: "Test User", Email: "test@example.com", ID: "user-123"},
				},
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `Opening browser for authentication...
Found space: space-123
Successfully logged in as test@example.com
`)
		assertOutput(t, result.stderr, "")
	})

	t.Run("success with new space created", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		emptySpaces := []api.Space{}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &emptySpaces,
			}, nil)
		postLoginMock.EXPECT().CreateSpaceWithResponse(validCtx, api.CreateSpaceJSONRequestBody{}).
			Return(&api.CreateSpaceResponse{
				HTTPResponse: httpResponse(http.StatusCreated),
				JSON201:      &api.Space{ID: "space-new", Name: "Ghost"},
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoTypeUser,
					User: &api.UserInfo{Name: "New User", Email: "new@example.com", ID: "user-456"},
				},
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `Opening browser for authentication...
Created space: space-new
Successfully logged in as new@example.com
`)
		assertOutput(t, result.stderr, "")
	})

	t.Run("invited new user creates owned space alongside joined space", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		// The user owns no space but has joined one (e.g. via invitation
		// auto-accepted at signup), so a personal space is created for them.
		joinedOnly := []api.Space{{ID: "space-joined", Name: "Team Space", Role: new(api.MemberRoleDeveloper)}}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &joinedOnly,
			}, nil)
		postLoginMock.EXPECT().CreateSpaceWithResponse(validCtx, api.CreateSpaceJSONRequestBody{}).
			Return(&api.CreateSpaceResponse{
				HTTPResponse: httpResponse(http.StatusCreated),
				JSON201:      &api.Space{ID: "space-new", Name: "New User's space", Role: new(api.MemberRoleOwner)},
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoTypeUser,
					User: &api.UserInfo{Name: "New User", Email: "new@example.com", ID: "user-456"},
				},
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `Opening browser for authentication...
Created space: space-new

You also belong to:
  Team Space (space-joined)
Run 'ghost space use <id>' to switch the current space.
Successfully logged in as new@example.com
`)
		assertOutput(t, result.stderr, "")
	})

	t.Run("existing user defaults to owned space and lists joined spaces", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		// The user owns a space and has also joined another; login defaults to
		// the owned one and points at `ghost space use` for the other.
		spaces := []api.Space{
			{ID: "space-owned", Name: "My Space", Role: new(api.MemberRoleOwner)},
			{ID: "space-joined", Name: "Team Space", Role: new(api.MemberRoleDeveloper)},
		}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &spaces,
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoTypeUser,
					User: &api.UserInfo{Name: "Test User", Email: "test@example.com", ID: "user-123"},
				},
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `Opening browser for authentication...
Found space: space-owned

You also belong to:
  Team Space (space-joined)
Run 'ghost space use <id>' to switch the current space.
Successfully logged in as test@example.com
`)
		assertOutput(t, result.stderr, "")
	})

	t.Run("headless login success with existing space", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		spaces := []api.Space{{ID: "space-123", Name: "My Space", Role: new(api.MemberRoleOwner)}}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &spaces,
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoTypeUser,
					User: &api.UserInfo{Name: "Test User", Email: "test@example.com", ID: "user-123"},
				},
			}, nil)

		result := runCommand(t, []string{"login", "--headless"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `
To authenticate, visit: https://github.com/login/device
and enter code: TEST-1234

Waiting for authorization (this can take a few seconds after you enter the code)...
Found space: space-123
Successfully logged in as test@example.com
`)
		assertOutput(t, result.stderr, "")
	})

	t.Run("headless login with new space", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		emptySpaces := []api.Space{}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &emptySpaces,
			}, nil)
		postLoginMock.EXPECT().CreateSpaceWithResponse(validCtx, api.CreateSpaceJSONRequestBody{}).
			Return(&api.CreateSpaceResponse{
				HTTPResponse: httpResponse(http.StatusCreated),
				JSON201:      &api.Space{ID: "space-new", Name: "Ghost"},
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200: &api.AuthInfo{
					Type: api.AuthInfoTypeUser,
					User: &api.UserInfo{Name: "New User", Email: "new@example.com", ID: "user-456"},
				},
			}, nil)

		result := runCommand(t, []string{"login", "--headless"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, `
To authenticate, visit: https://github.com/login/device
and enter code: TEST-1234

Waiting for authorization (this can take a few seconds after you enter the code)...
Created space: space-new
Successfully logged in as new@example.com
`)
		assertOutput(t, result.stderr, "")
	})

	t.Run("headless device code request fails", func(t *testing.T) {
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}))
		t.Cleanup(failServer.Close)

		result := runCommand(t, []string{"login", "--headless"}, nil,
			withEnv("GHOST_API_URL", failServer.URL),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(result.err.Error(), "failed to initiate device flow") {
			t.Errorf("expected error to contain 'failed to initiate device flow', got: %s", result.err.Error())
		}
	})

	t.Run("list spaces API error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusInternalServerError),
				JSONDefault:  &api.Error{Message: "internal server error"},
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to select space: failed to list spaces: internal server error")
	})

	t.Run("list spaces network error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(nil, errors.New("connection refused"))

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to select space: failed to list spaces: connection refused")
	})

	t.Run("list spaces nil response body", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      nil,
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to select space: failed to list spaces: empty response from API")
	})

	t.Run("create space API error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		emptySpaces := []api.Space{}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &emptySpaces,
			}, nil)
		postLoginMock.EXPECT().CreateSpaceWithResponse(validCtx, api.CreateSpaceJSONRequestBody{}).
			Return(&api.CreateSpaceResponse{
				HTTPResponse: httpResponse(http.StatusForbidden),
				JSONDefault:  &api.Error{Message: "forbidden"},
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to select space: failed to create space: forbidden")
	})

	t.Run("create space rejected because signups are disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		emptySpaces := []api.Space{}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &emptySpaces,
			}, nil)
		postLoginMock.EXPECT().CreateSpaceWithResponse(validCtx, api.CreateSpaceJSONRequestBody{}).
			Return(&api.CreateSpaceResponse{
				HTTPResponse: httpResponse(http.StatusForbidden),
				JSONDefault: &api.Error{
					Message: "sorry, Ghost is not accepting new users right now",
					Code:    new(api.ErrorCodeSignupsDisabled),
				},
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		// The API's message stands alone: no "failed to select space" prefixes.
		assertOutput(t, result.err.Error(), "sorry, Ghost is not accepting new users right now")
	})

	t.Run("create space nil response body", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		emptySpaces := []api.Space{}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &emptySpaces,
			}, nil)
		postLoginMock.EXPECT().CreateSpaceWithResponse(validCtx, api.CreateSpaceJSONRequestBody{}).
			Return(&api.CreateSpaceResponse{
				HTTPResponse: httpResponse(http.StatusCreated),
				JSON201:      nil,
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to select space: failed to create space: empty response from API")
	})

	t.Run("auth info API error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		spaces := []api.Space{{ID: "space-123", Name: "My Space", Role: new(api.MemberRoleOwner)}}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &spaces,
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusUnauthorized),
				JSONDefault:  &api.Error{Message: "unauthorized"},
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to identify user: unauthorized")
	})

	t.Run("auth info network error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		spaces := []api.Space{{ID: "space-123", Name: "My Space", Role: new(api.MemberRoleOwner)}}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &spaces,
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(nil, errors.New("connection refused"))

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to identify user: API call failed: connection refused")
	})

	t.Run("auth info nil response body", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		postLoginMock := mock.NewMockClientWithResponsesInterface(ctrl)

		spaces := []api.Space{{ID: "space-123", Name: "My Space", Role: new(api.MemberRoleOwner)}}
		postLoginMock.EXPECT().ListSpacesWithResponse(validCtx).
			Return(&api.ListSpacesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &spaces,
			}, nil)
		postLoginMock.EXPECT().AuthInfoWithResponse(validCtx).
			Return(&api.AuthInfoResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      nil,
			}, nil)

		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return postLoginMock, nil
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to identify user: empty response from API")
	})

	t.Run("new ghost client error", func(t *testing.T) {
		result := runCommand(t, []string{"login"}, nil,
			withEnv("GHOST_API_URL", tokenServer.URL),
			withOpenBrowser(mockBrowserForOAuth()),
			withNewGhostClient(func(apiURL string, auth api.AuthMethod) (api.ClientWithResponsesInterface, error) {
				return nil, errors.New("client creation failed")
			}),
		)

		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), "failed to create client: client creation failed")
	})
}
