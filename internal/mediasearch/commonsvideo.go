package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// CommonsVideo searches Wikimedia Commons for video — NASA visualisations,
// science animations, classroom explainers. It is the only video provider whose
// search results already contain a playable file URL. BaseURL is injectable for
// tests.
type CommonsVideo struct{ BaseURL string }

func (c CommonsVideo) ID() string      { return "commons-video" }
func (c CommonsVideo) Name() string    { return "Wikimedia Commons" }
func (c CommonsVideo) Kinds() []string { return []string{"video"} }

func (c CommonsVideo) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "video" {
		return nil, fmt.Errorf("mediasearch: commons-video: unsupported kind %q", kind)
	}
	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"gsrsearch":     {query + " filetype:video"},
		"gsrnamespace":  {"6"},
		"gsrlimit":      {strconv.Itoa(limit)},
		"prop":          {"imageinfo"},
		"iiprop":        {"url|mime|size"}, // size also yields duration
		"iiurlwidth":    {"320"},           // required, or imageinfo has no thumburl
	}
	var body llResponse
	if err := getJSON(ctx, c.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Query.Pages))
	for _, pg := range body.Query.Pages {
		if len(pg.ImageInfo) == 0 {
			continue // search can return pages whose imageinfo is unavailable
		}
		ii := pg.ImageInfo[0]
		out = append(out, Result{
			Kind:       "video",
			ThumbURL:   ii.ThumbURL,
			FullURL:    ii.URL,
			Title:      strings.TrimPrefix(pg.Title, "File:"),
			SourcePage: ii.DescriptionURL,
			Mime:       ii.Mime,
			DurationMs: int(ii.Duration * 1000),
		})
	}
	return out, nil
}

// ResolvePlayURL returns the URL unchanged: Commons search already supplies a
// direct file URL a <video> element can play.
func (c CommonsVideo) ResolvePlayURL(ctx context.Context, r Result) (string, error) {
	if r.FullURL == "" {
		return "", fmt.Errorf("mediasearch: commons-video: result has no file URL")
	}
	return r.FullURL, nil
}
