package cctv

import (
	"io/fs"
	"net/http"
)

// WebFS serves the built UI (ui/dist, gitignored). It is assigned by the main
// package, which embeds ui/dist at compile time:
//
//	pnpm --dir ui install && pnpm --dir ui build && go build .
//
// A missing ui/dist fails `go build` at compile time (unresolved embed
// pattern); a partial dist (no index.html) panics at startup via SetWebFS.
var WebFS fs.FS

// SetWebFS installs the embedded dist subtree and panics when index.html is
// absent so a broken frontend build fails fast instead of serving blanks.
func SetWebFS(f fs.FS) {
	if _, err := fs.Stat(f, "index.html"); err != nil {
		panic("cctv: ui/dist/index.html not found in embedded UI (run `pnpm --dir ui build` first): " + err.Error())
	}
	WebFS = f
}

// spaHandler serves the embedded UI, falling back to index.html for unknown
// paths so client-side routes keep working.
func spaHandler() http.Handler {
	fileServer := http.FileServer(http.FS(WebFS))
	// net/http's FileServer redirects */index.html to ./, so serve the entry
	// point bytes directly instead of routing them through it.
	index, err := fs.ReadFile(WebFS, "index.html")
	if err != nil {
		panic("cctv: ui/dist/index.html not readable: " + err.Error())
	}
	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := trimLeadingSlash(r.URL.Path)
		if p == "" || p == "index.html" {
			serveIndex(w)
			return
		}
		// Probe: serve index.html when the exact file is absent (or is a dir).
		fi, err := fs.Stat(WebFS, p)
		if err != nil || fi.IsDir() {
			serveIndex(w)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func trimLeadingSlash(p string) string {
	if len(p) > 0 && p[0] == '/' {
		return p[1:]
	}
	return p
}
