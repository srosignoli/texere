package media

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// remoteMaxBytes caps a downloaded image. A var, not a const, so tests can
// shrink it rather than transferring megabytes.
var remoteMaxBytes = 8 << 20 // 8 MB

// audioMaxBytes caps a downloaded audio file. A Lingua Libre clip is ~100 KB,
// but a general Commons recording can run for minutes. A var, not a const, so
// tests can shrink it.
var audioMaxBytes = 25 << 20 // 25 MB

const (
	remoteFetchTimeout = 15 * time.Second
	// remoteUserAgent identifies texere to remote servers. Go's default
	// ("Go-http-client/...") is rejected with 403 by Wikimedia and many other
	// hosts, and Commons' API policy requires an identifying agent.
	remoteUserAgent = "texere/1.0 (+https://github.com/srosignoli/texere) media-fetch"
)

var remoteClient = &http.Client{Timeout: remoteFetchTimeout}

// FetchRemote downloads rawURL, verifies it is a supported file of the given
// kind ("image" or "audio"), and stores it under <collectionDir>/media/
// content-addressed — so the same file fetched twice, from any URL, is stored
// once. The URL itself is not persisted here; the caller keeps provenance on
// the notebook cell.
func FetchRemote(collectionDir, rawURL, kind string) (Asset, error) {
	var maxBytes int
	switch kind {
	case "image":
		maxBytes = remoteMaxBytes
	case "audio":
		maxBytes = audioMaxBytes
	default:
		return Asset{}, fmt.Errorf("media: unsupported kind %q", kind)
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return Asset{}, fmt.Errorf("media: bad url: %w", err)
	}
	req.Header.Set("User-Agent", remoteUserAgent)
	resp, err := remoteClient.Do(req)
	if err != nil {
		return Asset{}, fmt.Errorf("media: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Asset{}, fmt.Errorf("media: fetch: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if err != nil {
		return Asset{}, fmt.Errorf("media: read: %w", err)
	}
	if len(data) > maxBytes {
		return Asset{}, fmt.Errorf("media: %s larger than %d MB", kind, maxBytes>>20)
	}

	var mime, ext string
	if kind == "image" {
		// Sniff rather than trust Content-Type: hotlink guards and CDNs
		// frequently mislabel, and Import() already makes sniffing the rule.
		mime = http.DetectContentType(data)
		var ok bool
		if ext, ok = extByMime[mime]; !ok {
			return Asset{}, fmt.Errorf("media: unsupported type %q", mime)
		}
	} else {
		// Audio is keyed by the URL's extension, never sniffed and never taken
		// from Content-Type: containers sniff unreliably, and Commons serves
		// .wav as "audio/x-wav", which is absent from audioMimeByExt.
		u, err := url.Parse(rawURL)
		if err != nil {
			return Asset{}, fmt.Errorf("media: bad url: %w", err)
		}
		ext = strings.ToLower(path.Ext(u.Path))
		var ok bool
		if mime, ok = audioMimeByExt[ext]; !ok {
			return Asset{}, fmt.Errorf("media: unsupported audio extension %q", ext)
		}
	}

	name, err := store(collectionDir, data, ext)
	if err != nil {
		return Asset{}, err
	}
	return Asset{Name: name, Mime: mime}, nil
}
