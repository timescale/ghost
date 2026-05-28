package serve

import (
	"embed"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed all:web
var embeddedAssets embed.FS

// webFS is the subtree of embeddedAssets rooted at "web/". Populated in init
// so each request handler doesn't need to repeat the fs.Sub call.
var webFS fs.FS

func init() {
	sub, err := fs.Sub(embeddedAssets, "web")
	if err != nil {
		panic(err)
	}
	webFS = sub
}

// hasBundledUI reports whether scripts/build-web.sh has been run and the
// resulting index.html is present in the embed. When false, the server falls
// back to a small placeholder page that tells the user how to build the UI.
func hasBundledUI() bool {
	_, err := fs.Stat(webFS, "index.html")
	return err == nil
}

const placeholderHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>ghost serve</title>
<style>body{font-family:system-ui,sans-serif;max-width:38rem;margin:4rem auto;padding:0 1rem;color:#0f172a}
code{background:#f1f5f9;padding:2px 6px;border-radius:4px;font-size:.9em}</style>
</head><body>
<h1>ghost serve</h1>
<p>The web UI bundle has not been built into this binary.</p>
<p>Run <code>./scripts/build-web.sh</code> from the repo root, then rebuild the binary.</p>
</body></html>
`

// newAssetHandler serves embedded SPA files. Behavior:
//   - "/" -> index.html
//   - exact match in webFS -> served with detected content type
//   - last segment has no "." -> SPA fallback to index.html
//   - otherwise -> 404
//
// Cache headers: /assets/* gets immutable + max-age=1y (Vite hashed files);
// everything else gets no-cache.
func newAssetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasBundledUI() {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte(placeholderHTML))
			return
		}

		urlPath := r.URL.Path
		if urlPath == "/" {
			urlPath = "/index.html"
		}
		clean := strings.TrimPrefix(path.Clean(urlPath), "/")

		data, err := fs.ReadFile(webFS, clean)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "asset read error", http.StatusInternalServerError)
				return
			}
			// SPA fallback: paths without an extension (no "." in last segment)
			// fall through to index.html so client-side routing works.
			last := path.Base(clean)
			if strings.Contains(last, ".") {
				http.NotFound(w, r)
				return
			}
			data, err = fs.ReadFile(webFS, "index.html")
			if err != nil {
				http.Error(w, "index.html missing from bundle", http.StatusInternalServerError)
				return
			}
			clean = "index.html"
		}

		ext := path.Ext(clean)
		contentType := mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)

		if strings.HasPrefix(clean, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}

		_, _ = w.Write(data)
	})
}
