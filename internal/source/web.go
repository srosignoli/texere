package source

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
	"github.com/microcosm-cc/bluemonday"
)

// WebArticle is a fetched, sanitized, self-contained article.
type WebArticle struct {
	Title string
	HTML  string
	URL   string
}

const (
	webFetchTimeout = 10 * time.Second
	webMaxBytes     = 10 << 20 // 10 MB
	imgFetchTimeout = 2 * time.Second
	imgMaxBytes     = 2 << 20 // 2 MB per image
)

// FetchAndExtract fetches url, extracts the readable article, sanitizes it, and
// inlines images as data: URIs so the result references no remote resources.
func FetchAndExtract(rawURL string) (WebArticle, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: bad url: %w", err)
	}
	client := &http.Client{Timeout: webFetchTimeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WebArticle{}, fmt.Errorf("source: fetch %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, webMaxBytes))
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: read body: %w", err)
	}

	article, err := readability.FromReader(bytes.NewReader(body), parsed)
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: readability: %w", err)
	}

	inlined := inlineImages(article.Content)
	policy := bluemonday.UGCPolicy()
	policy.AllowDataURIImages()
	safe := policy.Sanitize(inlined)

	return WebArticle{Title: strings.TrimSpace(article.Title), HTML: safe, URL: rawURL}, nil
}

// inlineImages downloads each <img src> and rewrites it to a data: URI; on any
// failure (or oversize) it drops the src so sanitization removes the <img>.
func inlineImages(htmlStr string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}
	imgClient := &http.Client{Timeout: imgFetchTimeout}
	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		src, ok := s.Attr("src")
		if !ok || strings.HasPrefix(src, "data:") {
			return
		}
		data, mime, err := fetchImage(imgClient, src)
		if err != nil {
			s.RemoveAttr("src")
			return
		}
		s.SetAttr("src", "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
	})
	if h, err := doc.Find("body").Html(); err == nil {
		return h
	}
	return htmlStr
}

func fetchImage(client *http.Client, src string) ([]byte, string, error) {
	resp, err := client.Get(src)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, imgMaxBytes+1))
	if err != nil || len(data) > imgMaxBytes {
		return nil, "", fmt.Errorf("image too large or unreadable")
	}
	mime := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", fmt.Errorf("not an image: %s", mime)
	}
	return data, mime, nil
}
