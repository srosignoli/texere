package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"texere/internal/llm"
)

type fakeProvider struct {
	chunks []string
	err    error
}

func (f fakeProvider) Stream(_ context.Context, _ llm.Request, onDelta func(string) error) error {
	for _, c := range f.chunks {
		if err := onDelta(c); err != nil {
			return err
		}
	}
	return f.err
}

// The reply arrives in chunks; the caller wants one string. Accumulating in Go
// keeps the frontend from reassembling deltas for a single value.
func TestTranslateQueryAccumulatesTheStream(t *testing.T) {
	got, err := translateWith(fakeProvider{chunks: []string{"pod ", "\\chapter ", "Kubernetes"}}, "m", "sys", "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `pod \chapter Kubernetes` {
		t.Fatalf("got %q, want the three chunks joined", got)
	}
}

func TestTranslateQuerySurfacesProviderErrors(t *testing.T) {
	_, err := translateWith(fakeProvider{chunks: []string{"x"}, err: errors.New("rate limited")}, "m", "sys", "q")
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err = %v, want the provider's error surfaced", err)
	}
}

func TestQueryLLMProfileRoundTrips(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &App{}
	if got := a.GetQueryLLMProfile(); got != "" {
		t.Fatalf("absent field should read as empty, got %q", got)
	}
	if err := a.SetQueryLLMProfile("prof-1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := a.GetQueryLLMProfile(); got != "prof-1" {
		t.Fatalf("got %q, want prof-1 to survive a reload", got)
	}
}
