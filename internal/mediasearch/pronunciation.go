package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Pronunciation searches Lingua Libre's recordings, which are hosted on
// Wikimedia Commons and filed under a Wikidata language code (LL-Q1321 =
// Spanish). BaseURL is injectable for tests.
//
// The language travels in the query as "lang|word" ("Spanish|manzana" or
// "Q1321|manzana") so the Provider interface stays uniform rather than growing
// a parameter only this provider uses.
type Pronunciation struct{ BaseURL string }

func (p Pronunciation) ID() string      { return "pronunciation" }
func (p Pronunciation) Name() string    { return "Pronunciation (Lingua Libre)" }
func (p Pronunciation) Kinds() []string { return []string{"audio"} }

// llResponse is the formatversion=2 shape with the audio fields. Commons
// reports duration in seconds as a float.
type llResponse struct {
	Query struct {
		Pages []struct {
			Title     string `json:"title"`
			ImageInfo []struct {
				URL            string  `json:"url"`
				DescriptionURL string  `json:"descriptionurl"`
				Mime           string  `json:"mime"`
				Duration       float64 `json:"duration"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

func (p Pronunciation) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "audio" {
		return nil, fmt.Errorf("mediasearch: pronunciation: unsupported kind %q", kind)
	}
	lang, word, ok := strings.Cut(query, "|")
	if !ok {
		return nil, fmt.Errorf("mediasearch: pronunciation: query must be \"language|word\", got %q", query)
	}
	word = strings.TrimSpace(word)
	if word == "" {
		return nil, fmt.Errorf("mediasearch: pronunciation: empty word")
	}
	code, ok := ResolveLanguage(lang)
	if !ok {
		return nil, fmt.Errorf("mediasearch: pronunciation: unknown language %q", lang)
	}

	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"gsrsearch":     {"LL-" + code + " " + word},
		"gsrnamespace":  {"6"}, // File:
		"gsrlimit":      {strconv.Itoa(limit)},
		"prop":          {"imageinfo"},
		"iiprop":        {"url|mime|size"}, // size also yields duration
	}
	var body llResponse
	if err := getJSON(ctx, p.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}

	all := make([]Result, 0, len(body.Query.Pages))
	exact := make([]Result, 0, len(body.Query.Pages))
	for _, pg := range body.Query.Pages {
		if len(pg.ImageInfo) == 0 {
			continue
		}
		ii := pg.ImageInfo[0]
		name := strings.TrimPrefix(pg.Title, "File:")
		lng, speaker, label := parseLinguaLibre(name, word)
		r := Result{
			Kind:       "audio",
			FullURL:    ii.URL,
			Title:      label,
			SourcePage: ii.DescriptionURL,
			Lang:       lng,
			Speaker:    speaker,
			Mime:       ii.Mime,
			DurationMs: int(ii.Duration * 1000),
		}
		all = append(all, r)
		if matchesWord(name, word) {
			exact = append(exact, r)
		}
	}
	// Prefer exact recordings of the word; fall back to everything the search
	// returned so a word with no exact recording still shows near-misses.
	if len(exact) > 0 {
		return exact, nil
	}
	return all, nil
}
