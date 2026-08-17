package media

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"texere/internal/collectionpath"
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
// route. This one needs directories, so it defends differently — via
// collectionpath.Resolve, which requires the result to stay under the root
// even through a symlink.
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
		full, err := collectionpath.Resolve(root, strings.TrimPrefix(r.URL.Path, prefix))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if ct, ok := audioOnlyMimeByExt[strings.ToLower(filepath.Ext(full))]; ok {
			w.Header().Set("Content-Type", ct)
		}
		http.ServeFile(w, r, full)
	})
}

// audioOnlyMimeByExt is audioMimeByExt without .mp4 and .webm.
//
// /media/ holds only imported images and recorded audio, so labelling those two
// containers audio there is correct. /collection/ serves whatever a note
// references, and mediaKind classifies both as VIDEO — so the same override
// would declare a <video>'s body to be audio. Go's own table already returns
// video/mp4 and video/webm, so they simply fall through.
var audioOnlyMimeByExt = map[string]string{
	".ogg": "audio/ogg", ".oga": "audio/ogg", ".wav": "audio/wav",
	".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".aac": "audio/aac",
	".flac": "audio/flac", ".opus": "audio/ogg",
}
