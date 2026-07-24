// Package media stores notebook image and audio assets on disk under
// <collectionDir>/media/ and serves them to the webview. Assets are
// content-addressed (sha256), so identical bytes are stored once. Pure standard
// library, no CGo.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Asset is a stored file: its filename under media/ and MIME type.
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

// audioMimeByExt maps a lowercased audio file extension to its MIME type
// (used by ImportAudio and the /media content-type on playback).
var audioMimeByExt = map[string]string{
	".webm": "audio/webm", ".ogg": "audio/ogg", ".oga": "audio/ogg",
	".wav": "audio/wav", ".mp3": "audio/mpeg", ".m4a": "audio/mp4",
	".mp4": "audio/mp4", ".aac": "audio/aac", ".flac": "audio/flac",
}

// audioExtByMime maps a normalized audio MIME (no codecs suffix) to an extension
// (used by SaveAudio for recordings).
var audioExtByMime = map[string]string{
	"audio/webm": ".webm", "audio/ogg": ".ogg", "audio/wav": ".wav",
	"audio/mpeg": ".mp3", "audio/mp4": ".m4a", "audio/aac": ".aac", "audio/flac": ".flac",
}

// store writes data to <collectionDir>/media/<sha256><ext> (dedup) and returns
// the filename. Shared by image and audio storage.
func store(collectionDir string, data []byte, ext string) (string, error) {
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ext
	dir := filepath.Join(collectionDir, "media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("media: mkdir: %w", err)
	}
	dst := filepath.Join(dir, name)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return "", fmt.Errorf("media: write: %w", err)
		}
	}
	return name, nil
}

// Import copies the image at srcPath into media/ (content-hashed). Non-image
// files are rejected (MIME sniffed).
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
	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: mime}, nil
}

// ImportAudio copies the audio file at srcPath into media/, keyed by its file
// extension (audio containers sniff unreliably). Rejects non-audio extensions.
func ImportAudio(collectionDir, srcPath string) (Asset, error) {
	ext := strings.ToLower(filepath.Ext(srcPath))
	mime, ok := audioMimeByExt[ext]
	if !ok {
		return Asset{}, fmt.Errorf("media: unsupported audio extension %q", ext)
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return Asset{}, fmt.Errorf("media: read source: %w", err)
	}
	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: mime}, nil
}

// SaveAudio stores recorded audio bytes. mime may carry a codecs suffix
// (e.g. "audio/webm;codecs=opus"), which is stripped to select the extension.
func SaveAudio(collectionDir string, data []byte, mime string) (Asset, error) {
	base := mime
	if i := strings.IndexByte(base, ';'); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	ext, ok := audioExtByMime[base]
	if !ok {
		return Asset{}, fmt.Errorf("media: unsupported audio mime %q", mime)
	}
	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: base}, nil
}
