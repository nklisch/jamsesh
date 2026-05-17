// Package assets embeds the compiled Svelte SPA and serves it with a
// History-API fallback so that deep links (e.g. /orgs/foo/sessions) resolve
// to index.html client-side.
//
// The embed directive uses `all:dist` to include the directory even when only
// a .gitkeep is present (the `all:` prefix includes hidden/dot files and
// prevents a "no matching files" compile error on a fresh checkout where
// `npm run build` hasn't run yet).  On every real build, `make frontend-build`
// runs before `go build`, so dist/ is fully populated.
package assets

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler returns an http.Handler that serves the embedded SPA.
// Requests for files that exist (JS/CSS bundles, assets) are served directly.
// Everything else falls back to index.html so the History-API router in the
// browser can resolve the path client-side.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Attempt to open the requested path in the embedded FS.
		// Strip leading slash to match the fs.Sub root.
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if f, err := sub.Open(name); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// File not found — serve index.html for SPA routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}), nil
}
