package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// commonsAPI is the live MediaWiki endpoint. Commons is strongest for concrete
// nouns — good for vocabulary, thin for abstract words.
const commonsAPI = "https://commons.wikimedia.org/w/api.php"

// Commons searches Wikimedia Commons. BaseURL is injectable for tests; the zero
// value is not usable — construct via Providers() or with BaseURL set.
type Commons struct{ BaseURL string }

func (c Commons) ID() string      { return "commons" }
func (c Commons) Name() string    { return "Wikimedia Commons" }
func (c Commons) Kinds() []string { return []string{"image"} }

// commonsResponse is the formatversion=2 shape, where "pages" is a JSON array
// rather than the pageid-keyed object of formatversion=1.
type commonsResponse struct {
	Query struct {
		Pages []struct {
			Title     string `json:"title"`
			ImageInfo []struct {
				ThumbURL       string `json:"thumburl"`
				URL            string `json:"url"`
				DescriptionURL string `json:"descriptionurl"`
				ExtMetadata    struct {
					LicenseShortName struct {
						Value string `json:"value"`
					} `json:"LicenseShortName"`
				} `json:"extmetadata"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

func (c Commons) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "image" {
		return nil, fmt.Errorf("mediasearch: commons: unsupported kind %q", kind)
	}
	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"gsrsearch":     {query},
		"gsrnamespace":  {"6"}, // File:
		"gsrlimit":      {strconv.Itoa(limit)},
		"prop":          {"imageinfo"},
		"iiprop":        {"url|extmetadata"},
		"iiurlwidth":    {"240"}, // asks for a thumburl at this width
	}
	var body commonsResponse
	if err := getJSON(ctx, c.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Query.Pages))
	for _, p := range body.Query.Pages {
		if len(p.ImageInfo) == 0 {
			continue // search can return pages whose imageinfo is unavailable
		}
		ii := p.ImageInfo[0]
		out = append(out, Result{
			Kind:       "image",
			ThumbURL:   ii.ThumbURL,
			FullURL:    ii.URL,
			Title:      strings.TrimPrefix(p.Title, "File:"),
			SourcePage: ii.DescriptionURL,
			License:    ii.ExtMetadata.LicenseShortName.Value,
		})
	}
	return out, nil
}
