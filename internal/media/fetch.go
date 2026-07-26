package media

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// remoteMaxBytes caps a downloaded image. A var, not a const, so tests can
// shrink it rather than transferring megabytes.
var remoteMaxBytes = 8 << 20 // 8 MB

const (
	remoteFetchTimeout = 15 * time.Second
	// remoteUserAgent identifies texere to remote servers. Go's default
	// ("Go-http-client/...") is rejected with 403 by Wikimedia and many other
	// hosts, and Commons' API policy requires an identifying agent.
	remoteUserAgent = "texere/1.0 (+https://github.com/srosignoli/texere) media-fetch"
)

var remoteClient = &http.Client{Timeout: remoteFetchTimeout}

// FetchRemote downloads rawURL, verifies it really is a supported image, and
// stores it under <collectionDir>/media/ content-addressed — so the same picture
// fetched twice, from any URL, is stored once. The URL itself is not persisted
// here; the caller keeps provenance in the notebook cell.
func FetchRemote(collectionDir, rawURL string) (Asset, error) {
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(remoteMaxBytes)+1))
	if err != nil {
		return Asset{}, fmt.Errorf("media: read: %w", err)
	}
	if len(data) > remoteMaxBytes {
		return Asset{}, fmt.Errorf("media: image larger than %d MB", remoteMaxBytes>>20)
	}
	// Sniff rather than trust Content-Type: hotlink guards and CDNs frequently
	// mislabel, and Import() already establishes sniffing as the house rule.
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
