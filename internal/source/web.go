package source

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
	// userAgent identifies texere to remote servers. The default Go
	// User-Agent ("Go-http-client/...") is rejected with 403 by Wikipedia and
	// many other sites, so we send a descriptive one instead.
	userAgent = "texere/1.0 (+https://github.com/srosignoli/texere) reading-importer"
)

// getWithUA performs a GET with the texere User-Agent header set.
func getWithUA(client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return client.Do(req)
}

// FetchAndExtract fetches url, extracts the readable article, sanitizes it, and
// inlines images as data: URIs so the result references no remote resources.
func FetchAndExtract(rawURL string) (WebArticle, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return WebArticle{}, fmt.Errorf("source: bad url: %w", err)
	}
	client := &http.Client{Timeout: webFetchTimeout}
	resp, err := getWithUA(client, rawURL)
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

// imgConcurrency bounds how many images are fetched in parallel during inlining.
const imgConcurrency = 6

// inlineImages downloads each <img src> and rewrites it to a data: URI; on any
// failure (or oversize) it drops the src so sanitization removes the <img>. The
// downloads are independent, so they run concurrently (bounded) — otherwise an
// N-image article blocks for up to N*imgFetchTimeout serially. goquery is not
// safe for concurrent mutation, so fetches run in parallel but the results are
// applied to the DOM on this goroutine.
func inlineImages(htmlStr string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}
	imgClient := &http.Client{Timeout: imgFetchTimeout}

	var sels []*goquery.Selection
	var srcs []string
	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		src, ok := s.Attr("src")
		if !ok || strings.HasPrefix(src, "data:") {
			return
		}
		sels = append(sels, s)
		srcs = append(srcs, src)
	})

	type fetched struct {
		data []byte
		mime string
		err  error
	}
	results := make([]fetched, len(srcs))
	sem := make(chan struct{}, imgConcurrency)
	var wg sync.WaitGroup
	for i, src := range srcs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, src string) {
			defer wg.Done()
			defer func() { <-sem }()
			data, mime, err := fetchImage(imgClient, src)
			results[i] = fetched{data, mime, err}
		}(i, src)
	}
	wg.Wait()

	for i, s := range sels {
		if r := results[i]; r.err != nil {
			s.RemoveAttr("src")
		} else {
			s.SetAttr("src", "data:"+r.mime+";base64,"+base64.StdEncoding.EncodeToString(r.data))
		}
	}
	if h, err := doc.Find("body").Html(); err == nil {
		return h
	}
	return htmlStr
}

func fetchImage(client *http.Client, src string) ([]byte, string, error) {
	resp, err := getWithUA(client, src)
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
