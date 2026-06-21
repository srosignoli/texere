// Package source produces the canonical text "projection" a study source is
// read and extracted from. Anchors in (from) strings are char offsets into this
// projection. Phase 2a handles local text/HTML; PDF and web URLs are later
// adapters behind the same Projection entry point.
package source

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

// Projection returns the text the reader renders and anchors into.
func Projection(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		return htmlToText(data)
	default:
		return string(data), nil
	}
}

// htmlToText extracts visible text, dropping <script>/<style> content and
// collapsing all whitespace to single spaces (deterministic so char anchors are
// stable).
func htmlToText(data []byte) (string, error) {
	z := html.NewTokenizer(bytes.NewReader(data))
	var b strings.Builder
	skip := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break // io.EOF terminates the token stream
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			if n := string(name); n == "script" || n == "style" {
				skip++
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if n := string(name); (n == "script" || n == "style") && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " "), nil
}
