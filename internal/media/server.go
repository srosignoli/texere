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

// CollectionServer serves files anywhere under the collection directory at
// /collection/<relative-path>. dirFn returns the current collection directory
// (empty when none is open).
//
// Separate from FileServer rather than a widening of it: FileServer's collapse
// to a base name IS its traversal defence, and the notebook depends on that
// route. This one needs directories, so it defends differently — resolve, then
// require the result to stay under the root.
//
// That containment is stricter than GetSourceBytes, which cleans ".." and will
// happily read outside the collection. Tightening it here is deliberate.
func CollectionServer(dirFn func() string) http.Handler {
	const prefix = "/collection/"
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
		root, err := filepath.Abs(dir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		// filepath.Join cleans "..", so the result may sit outside root; the
		// prefix check below is what actually enforces containment.
		full := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, prefix)))
		if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
			http.NotFound(w, r)
			return
		}
		if ct, ok := audioMimeByExt[strings.ToLower(filepath.Ext(full))]; ok {
			w.Header().Set("Content-Type", ct) // Go calls .webm video/webm; be explicit for <audio>
		}
		http.ServeFile(w, r, full)
	})
}
