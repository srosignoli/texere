package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth header = %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := &openaiProvider{baseURL: srv.URL, apiKey: "k"}
	var sb strings.Builder
	err := p.Stream(context.Background(),
		Request{Model: "m", System: "sys", Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 16},
		func(s string) error { sb.WriteString(s); return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if sb.String() != "Hello" {
		t.Fatalf("got %q, want Hello", sb.String())
	}
}

func TestOpenAIStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	p := &openaiProvider{baseURL: srv.URL, apiKey: "k"}
	err := p.Stream(context.Background(), Request{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("want error containing 'bad key', got %v", err)
	}
}
