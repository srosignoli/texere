package mediasearch

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strconv"
)

const (
	archiveSearchAPI   = "https://archive.org/advancedsearch.php"
	archiveMetadataAPI = "https://archive.org/metadata"
)

// InternetArchive searches the Internet Archive's moving-image collections —
// university lecture webcasts, public-domain film, science material. BaseURL and
// MetadataBase are injectable for tests.
//
// Its corpus is uneven even after filtering (the top hit for "photosynthesis" is
// a YouTube mirror); that is a characteristic of the source, not a defect.
type InternetArchive struct {
	BaseURL      string
	MetadataBase string
}

func (i InternetArchive) ID() string      { return "archive" }
func (i InternetArchive) Name() string    { return "Internet Archive" }
func (i InternetArchive) Kinds() []string { return []string{"video"} }

type archiveSearchResponse struct {
	Response struct {
		Docs []struct {
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
		} `json:"docs"`
	} `json:"response"`
}

func (i InternetArchive) Search(ctx context.Context, query, kind string, limit int) ([]Result, error) {
	if kind != "video" {
		return nil, fmt.Errorf("mediasearch: archive: unsupported kind %q", kind)
	}
	q := url.Values{
		// The tvarchive exclusion is load-bearing: without it the TV News
		// Archive dominates the results entirely.
		"q":      {query + " AND mediatype:movies AND NOT collection:tvarchive"},
		"rows":   {strconv.Itoa(limit)},
		"output": {"json"},
	}
	q["fl[]"] = []string{"identifier", "title"}

	var body archiveSearchResponse
	if err := getJSON(ctx, i.BaseURL+"?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Response.Docs))
	for _, d := range body.Response.Docs {
		if d.Identifier == "" {
			continue
		}
		out = append(out, Result{
			Kind:       "video",
			ThumbURL:   "https://archive.org/services/img/" + d.Identifier,
			Title:      d.Title,
			SourcePage: "https://archive.org/details/" + d.Identifier,
			// FullURL stays empty: resolved lazily on pick.
		})
	}
	return out, nil
}

type archiveMetadata struct {
	Files []struct {
		Name   string `json:"name"`
		Format string `json:"format"`
	} `json:"files"`
}

// archivePlayableFormats are the metadata `format` values a <video> element can
// play, in preference order.
var archivePlayableFormats = []string{"h.264", "MPEG4", "512Kb MPEG4", "WebM", "Ogg Video"}

// ResolvePlayURL fetches the item's file list and returns a download URL for the
// most preferred playable format. archive.org/download answers 302 to a CDN
// node, which a <video> element follows.
func (i InternetArchive) ResolvePlayURL(ctx context.Context, r Result) (string, error) {
	id, err := archiveIdentifier(r.SourcePage)
	if err != nil {
		return "", err
	}
	base := i.MetadataBase
	if base == "" {
		base = archiveMetadataAPI
	}
	var m archiveMetadata
	if err := getJSON(ctx, base+"/"+id, &m); err != nil {
		return "", fmt.Errorf("mediasearch: archive: %w", err)
	}
	for _, want := range archivePlayableFormats {
		for _, f := range m.Files {
			if f.Format == want && f.Name != "" {
				return "https://archive.org/download/" + id + "/" + f.Name, nil
			}
		}
	}
	return "", fmt.Errorf("mediasearch: archive: no playable file for %q", id)
}

// archiveIdentifier extracts the item id from a details page URL:
// https://archive.org/details/<identifier>
func archiveIdentifier(detailsURL string) (string, error) {
	u, err := url.Parse(detailsURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("mediasearch: archive: unusable details url %q", detailsURL)
	}
	id := path.Base(u.Path)
	if id == "" || id == "." || id == "/" || id == "details" {
		return "", fmt.Errorf("mediasearch: archive: no identifier in %q", detailsURL)
	}
	return id, nil
}
