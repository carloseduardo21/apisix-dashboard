package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the embedded frontend, falling back to index.html for
// client-side routes.
func spaHandler(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "api.method_not_allowed", "method not allowed")
			return
		}
		if _, err := fs.Stat(fsys, "index.html"); err != nil {
			http.Error(w, "frontend not included in this binary (run make build)", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
			return
		}
		if fi, err := fs.Stat(fsys, name); err != nil || fi.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, fsys, "index.html")
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
