// Package media stores notebook image assets on disk under <collectionDir>/media/
// and serves them to the webview. Assets are content-addressed (sha256), so the
// same image imported twice is stored once. Pure standard library, no CGo.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// Asset is a stored image: its filename under media/ and detected MIME type.
type Asset struct {
	Name string
	Mime string
}

var extByMime = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// Import copies the image at srcPath into <collectionDir>/media/ under a
// content-hashed name (<sha256>.<ext>) and returns its reference. Identical bytes
// yield the same name and are written once. Non-image files are rejected.
func Import(collectionDir, srcPath string) (Asset, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return Asset{}, fmt.Errorf("media: read source: %w", err)
	}
	mime := http.DetectContentType(data)
	ext, ok := extByMime[mime]
	if !ok {
		return Asset{}, fmt.Errorf("media: unsupported type %q", mime)
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ext
	dir := filepath.Join(collectionDir, "media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Asset{}, fmt.Errorf("media: mkdir: %w", err)
	}
	dst := filepath.Join(dir, name)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return Asset{}, fmt.Errorf("media: write: %w", err)
		}
	}
	return Asset{Name: name, Mime: mime}, nil
}
