package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strconv"
)

// sepiaSearchAPI federates search across PeerTube instances, including
// education-ministry ones. Keyless.
const sepiaSearchAPI = "https://sepiasearch.org/api/v1/search/videos"

// PeerTube searches the PeerTube federation through SepiaSearch. Search returns
// only a watch page; the playable file URL is fetched on demand by
// ResolvePlayURL. BaseURL is the search endpoint and DetailBase, when set,
// overrides the per-instance detail endpoint — both are injectable for tests.
type PeerTube struct {
	BaseURL    string
	DetailBase string
}

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

type peertubeDetail struct {
	Files []struct {
		FileURL    string `json:"fileUrl"`
		Resolution struct {
			ID int `json:"id"`
		} `json:"resolution"`
	} `json:"files"`
	StreamingPlaylists []struct {
		PlaylistURL string `json:"playlistUrl"`
	} `json:"streamingPlaylists"`
}

// ResolvePlayURL fetches the video's file list from its own instance and returns
// the highest-resolution progressive file. Host and uuid are recovered from the
// watch URL, so no extra Result fields are needed.
//
// Instances that publish only HLS (.m3u8) cannot be played by a bare <video>
// element and this slice ships no hls.js, so they are reported plainly rather
// than yielding an empty URL that would fail later and further away.
func (p PeerTube) ResolvePlayURL(ctx context.Context, r Result) (string, error) {
	host, uuid, err := peertubeWatchParts(r.SourcePage)
	if err != nil {
		return "", err
	}
	base := p.DetailBase
	if base == "" {
		base = "https://" + host + "/api/v1/videos"
	}
	var d peertubeDetail
	if err := getJSON(ctx, base+"/"+uuid, &d); err != nil {
		return "", fmt.Errorf("mediasearch: peertube: %w", err)
	}
	best, bestRes := "", -1
	for _, f := range d.Files {
		if f.FileURL != "" && f.Resolution.ID > bestRes {
			best, bestRes = f.FileURL, f.Resolution.ID
		}
	}
	if best == "" {
		return "", fmt.Errorf("mediasearch: peertube: no playable file (this instance publishes HLS only)")
	}
	return best, nil
}

// peertubeWatchParts splits a PeerTube watch URL into its instance host and
// video uuid: https://diler.tube/videos/watch/<uuid>
func peertubeWatchParts(watchURL string) (host, uuid string, err error) {
	u, err := url.Parse(watchURL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("mediasearch: peertube: unusable watch url %q", watchURL)
	}
	id := path.Base(u.Path)
	if id == "" || id == "." || id == "/" || id == "watch" {
		return "", "", fmt.Errorf("mediasearch: peertube: no video id in %q", watchURL)
	}
	return u.Host, id, nil
}
