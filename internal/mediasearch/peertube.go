package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// sepiaSearchAPI federates search across PeerTube instances, including
// education-ministry ones. Keyless.
const sepiaSearchAPI = "https://sepiasearch.org/api/v1/search/videos"

// PeerTube searches the PeerTube federation through SepiaSearch. Search returns
// only a watch page; the playable file URL is fetched on demand by
// ResolvePlayURL. BaseURL is injectable for tests.
type PeerTube struct{ BaseURL string }

func (p PeerTube) ID() string      { return "peertube" }
func (p PeerTube) Name() string    { return "PeerTube" }
func (p PeerTube) Kinds() []string { return []string{"video"} }

type sepiaResponse struct {
	Data []struct {
		UUID          string `json:"uuid"`
		Name          string `json:"name"`
		Duration      int    `json:"duration"` // seconds
		ThumbnailPath string `json:"thumbnailPath"`
		URL           string `json:"url"`
		Account       struct {
			Host string `json:"host"`
		} `json:"account"`
	} `json:"data"`
}

func (p PeerTube) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "video" {
		return nil, fmt.Errorf("mediasearch: peertube: unsupported kind %q", kind)
	}
	q := url.Values{
		"search": {query},
		"count":  {strconv.Itoa(limit)},
	}
	var body sepiaResponse
	if err := getJSON(ctx, p.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Data))
	for _, v := range body.Data {
		// thumbnailPath is relative to the hosting instance. Without the host it
		// would resolve against the app's own origin and render nothing, so an
		// unknown host yields no thumbnail rather than a broken one.
		thumb := ""
		if v.Account.Host != "" && v.ThumbnailPath != "" {
			thumb = "https://" + v.Account.Host + v.ThumbnailPath
		}
		out = append(out, Result{
			Kind:       "video",
			ThumbURL:   thumb,
			Title:      v.Name,
			SourcePage: v.URL,
			DurationMs: v.Duration * 1000,
			// FullURL stays empty: resolved lazily on pick.
		})
	}
	return out, nil
}
