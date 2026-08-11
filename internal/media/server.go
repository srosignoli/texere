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
		full, ok := resolveUnder(root, strings.TrimPrefix(r.URL.Path, prefix))
		if !ok {
			http.NotFound(w, r)
			return
		}
		if ct, ok := audioMimeByExt[strings.ToLower(filepath.Ext(full))]; ok {
			w.Header().Set("Content-Type", ct) // Go calls .webm video/webm; be explicit for <audio>
		}
		http.ServeFile(w, r, full)
	})
}

// resolveUnder joins rel onto root and reports whether the result stayed inside
// root. filepath.Join cleans "..", so the join alone proves nothing — the
// prefix check is what enforces containment.
//
// Split out of the handler so it can be tested at all. http.ServeFile
// independently rejects any request whose URL path contains a ".." element
// (net/http/fs.go:815), so no request reaching CollectionServer can exercise
// this: it is defence in depth for a caller whose path did not come from a URL,
// and a handler-level test of it would pass with the check deleted.
func resolveUnder(root, rel string) (string, bool) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}
