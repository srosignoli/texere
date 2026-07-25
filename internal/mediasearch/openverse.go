package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// openverseAPI works anonymously (rate-limited); registering a key raises the
// limits and is a later slice's concern.
const openverseAPI = "https://api.openverse.org/v1/images/"

// Openverse aggregates openly-licensed images from Flickr and similar sources.
// Broader everyday coverage than Commons. BaseURL is injectable for tests.
type Openverse struct{ BaseURL string }

func (o Openverse) ID() string      { return "openverse" }
func (o Openverse) Name() string    { return "Openverse" }
func (o Openverse) Kinds() []string { return []string{"image"} }

type openverseResponse struct {
	Results []struct {
		Title             string `json:"title"`
		URL               string `json:"url"`
		Thumbnail         string `json:"thumbnail"`
		ForeignLandingURL string `json:"foreign_landing_url"`
		License           string `json:"license"`
		LicenseVersion    string `json:"license_version"`
	} `json:"results"`
}

func (o Openverse) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "image" {
		return nil, fmt.Errorf("mediasearch: openverse: unsupported kind %q", kind)
	}
	q := url.Values{
		"q":         {query},
		"page_size": {strconv.Itoa(limit)},
	}
	var body openverseResponse
	if err := getJSON(ctx, o.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Results))
	for _, r := range body.Results {
		out = append(out, Result{
			Kind:       "image",
			ThumbURL:   r.Thumbnail,
			FullURL:    r.URL,
			Title:      r.Title,
			SourcePage: r.ForeignLandingURL,
			License:    openverseLicense(r.License, r.LicenseVersion),
		})
	}
	return out, nil
}

// openverseLicense renders Openverse's ("by-sa", "4.0") into "CC BY-SA 4.0",
// omitting the version when absent so no trailing space is produced.
func openverseLicense(code, version string) string {
	if code == "" {
		return ""
	}
	s := "CC " + strings.ToUpper(code)
	if version != "" {
		s += " " + version
	}
	return s
}
