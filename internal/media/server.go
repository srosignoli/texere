package media

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// FileServer serves files under the media directory at /media/<name>. dirFn
// returns the current media directory (empty when no collection is open). It is
// path-traversal safe: only a sanitized base name under dirFn() is ever served.
func FileServer(dirFn func() string) http.Handler {
	const prefix = "/media/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		dir := dirFn()
		if dir == "" {
			http.NotFound(w, r)
			return
		}
		name := path.Base(r.URL.Path) // collapses any ".." / directory components
		if name == "." || name == ".." || name == "/" || strings.ContainsRune(name, filepath.Separator) {
			http.NotFound(w, r)
			return
		}
		if ct, ok := audioMimeByExt[strings.ToLower(filepath.Ext(name))]; ok {
			w.Header().Set("Content-Type", ct) // Go's mime table calls .webm video/webm; be explicit for <audio>
		}
		http.ServeFile(w, r, filepath.Join(dir, name))
	})
}
