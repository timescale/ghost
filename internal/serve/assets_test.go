package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssetHandler_PlaceholderWhenNoBundle(t *testing.T) {
	// internal/serve/web/ contains only .gitkeep when this test runs in a
	// fresh checkout; hasBundledUI() returns false and the placeholder
	// page is served.
	if hasBundledUI() {
		t.Skip("web bundle is present; placeholder behavior is exercised in fresh checkouts")
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	newAssetHandler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html...", ct)
	}
	if !strings.Contains(w.Body.String(), "build-web.sh") {
		t.Errorf("placeholder body missing build-web.sh hint:\n%s", w.Body.String())
	}
}

func TestAssetHandler_RealBundleSPABehavior(t *testing.T) {
	if !hasBundledUI() {
		t.Skip("requires built web bundle (run scripts/build-web.sh)")
	}
	h := newAssetHandler()

	cases := []struct {
		name       string
		path       string
		wantStatus int
		wantCache  string
		wantBody   string
	}{
		{
			name:       "root serves index.html",
			path:       "/",
			wantStatus: http.StatusOK,
			wantCache:  "no-cache",
			wantBody:   "<!doctype html>",
		},
		{
			name:       "SPA fallback for paths without extension",
			path:       "/some/route",
			wantStatus: http.StatusOK,
			wantCache:  "no-cache",
			wantBody:   "<!doctype html>",
		},
		{
			name:       "missing dotted asset is 404",
			path:       "/missing.png",
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			h.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.wantCache != "" && w.Header().Get("Cache-Control") != tc.wantCache {
				t.Errorf("Cache-Control = %q, want %q", w.Header().Get("Cache-Control"), tc.wantCache)
			}
			if tc.wantBody != "" && !strings.Contains(strings.ToLower(w.Body.String()), tc.wantBody) {
				t.Errorf("body missing %q:\n%.300s", tc.wantBody, w.Body.String())
			}
		})
	}
}
