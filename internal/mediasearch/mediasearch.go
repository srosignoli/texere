// Package mediasearch searches the web for study assets (images now; audio and
// video later) and normalizes every provider's response into a common Result.
// It knows nothing about collections or storage — the caller downloads a chosen
// FullURL via media.FetchRemote. Pure standard library, no CGo.
package mediasearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Result is one search hit, normalized across providers. Field tags match the
// camelCase the Svelte frontend expects from the Wails binding.
type Result struct {
	Kind       string `json:"kind"`     // "image" | "audio"
	ThumbURL   string `json:"thumbUrl"` // small preview for the results grid (images)
	FullURL    string `json:"fullUrl"`  // what gets downloaded on pick
	Title      string `json:"title"`
	SourcePage string `json:"sourcePage"` // human-facing page, for attribution
	License    string `json:"license"`

	// Audio-only; empty/zero for images.
	Lang       string `json:"lang"`       // ISO code parsed from the filename, e.g. "spa"
	Speaker    string `json:"speaker"`    // contributor who made the recording
	Mime       string `json:"mime"`       // e.g. "audio/wav"; drives the format label
	DurationMs int    `json:"durationMs"` // 0 when unknown
}

// Provider is one searchable source. Implementations must be safe for
// concurrent use and must honour ctx cancellation, because the UI cancels a
// search as soon as the query changes.
type Provider interface {
	ID() string      // stable identifier persisted in the cell
	Name() string    // display label
	Kinds() []string // kinds this provider can serve
	Search(ctx context.Context, query, kind string, limit int) ([]Result, error)
}

// VideoResolver is implemented by video providers whose search results do not
// already carry a playable URL. Resolution is deliberately lazy — SepiaSearch
// returns only a watch page and the Internet Archive only an identifier, so
// resolving every result eagerly would mean dozens of extra requests per
// search instead of one per pick.
type VideoResolver interface {
	ResolvePlayURL(ctx context.Context, r Result) (string, error)
}

const (
	searchTimeout  = 15 * time.Second
	searchMaxBytes = 4 << 20 // 4 MB of JSON is far beyond any sane response
	// errBodyMaxBytes caps how much of a failed response is quoted back in the
	// error — enough to carry the provider's explanation, not a whole page.
	errBodyMaxBytes = 512
	// searchUserAgent identifies texere to remote APIs. Go's default is 403'd
	// by Wikimedia, whose API policy requires an identifying agent.
	searchUserAgent = "texere/1.0 (+https://github.com/srosignoli/texere) media-search"
)

var searchClient = &http.Client{Timeout: searchTimeout}

// getJSON performs a GET with the texere User-Agent and decodes the response
// into out. Non-2xx responses are errors.
func getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("mediasearch: bad url: %w", err)
	}
	req.Header.Set("User-Agent", searchUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := searchClient.Do(req)
	if err != nil {
		return fmt.Errorf("mediasearch: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Include the body: providers explain the refusal there, and a bare
		// status code is undiagnosable from the UI. Openverse, for one, answers
		// an over-large page_size with 401 and a body naming the real cause.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyMaxBytes))
		if s := strings.TrimSpace(string(snippet)); s != "" {
			return fmt.Errorf("mediasearch: status %d: %s", resp.StatusCode, s)
		}
		return fmt.Errorf("mediasearch: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, searchMaxBytes))
	if err != nil {
		return fmt.Errorf("mediasearch: read: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("mediasearch: decode: %w", err)
	}
	return nil
}
