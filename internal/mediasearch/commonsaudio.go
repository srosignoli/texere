package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// CommonsAudio searches Wikimedia Commons for any audio — sound effects, animal
// calls, music. It applies no word filtering; for word pronunciations use
// Pronunciation, whose shaped query is what makes those findable at all.
// BaseURL is injectable for tests.
type CommonsAudio struct{ BaseURL string }

func (c CommonsAudio) ID() string      { return "commons-audio" }
func (c CommonsAudio) Name() string    { return "Commons audio" }
func (c CommonsAudio) Kinds() []string { return []string{"audio"} }

func (c CommonsAudio) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "audio" {
		return nil, fmt.Errorf("mediasearch: commons-audio: unsupported kind %q", kind)
	}
	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"gsrsearch":     {query + " filetype:audio"},
		"gsrnamespace":  {"6"},
		"gsrlimit":      {strconv.Itoa(limit)},
		"prop":          {"imageinfo"},
		"iiprop":        {"url|mime|size"},
	}
	var body llResponse
	if err := getJSON(ctx, c.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Query.Pages))
	for _, pg := range body.Query.Pages {
		if len(pg.ImageInfo) == 0 {
			continue
		}
		ii := pg.ImageInfo[0]
		name := strings.TrimPrefix(pg.Title, "File:")
		// These are ordinary Commons names, not Lingua Libre ones, so no
		// language or speaker is inferred and the label is simply the filename
		// without its extension. Lang and Speaker are deliberately left empty.
		label := strings.TrimSuffix(name, path.Ext(name))
		out = append(out, Result{
			Kind:       "audio",
			FullURL:    ii.URL,
			Title:      label,
			SourcePage: ii.DescriptionURL,
			Mime:       ii.Mime,
			DurationMs: int(ii.Duration * 1000),
		})
	}
	return out, nil
}
