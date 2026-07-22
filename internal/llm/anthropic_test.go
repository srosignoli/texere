package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "k" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n")
		io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}\n\n")
		io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	p := &anthropicProvider{baseURL: srv.URL, apiKey: "k"}
	var sb strings.Builder
	err := p.Stream(context.Background(),
		Request{Model: "claude-opus-4-8", System: "sys", Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 16},
		func(s string) error { sb.WriteString(s); return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if sb.String() != "Hello" {
		t.Fatalf("got %q, want Hello", sb.String())
	}
}

func TestAnthropicStreamErrorEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"overloaded\"}}\n\n")
	}))
	defer srv.Close()
	p := &anthropicProvider{baseURL: srv.URL, apiKey: "k"}
	err := p.Stream(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Fatalf("want error containing 'overloaded', got %v", err)
	}
}
