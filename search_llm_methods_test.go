package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	got, err := translateWith(fakeProvider{chunks: []string{"x"}, err: errors.New("rate limited")}, "m", "sys", "q")
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err = %v, want the provider's error surfaced", err)
	}
	// A partial reply must not reach the caller as if it were a finished query:
	// it would be written straight into the query bar looking complete.
	if got != "" {
		t.Fatalf("got %q alongside the error, want the partial reply discarded", got)
	}
}

// A provider that hangs until the context ends. Without a deadline this test
// would block forever, which is precisely the bug.
type hangingProvider struct{}

func (hangingProvider) Stream(ctx context.Context, _ llm.Request, _ func(string) error) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestTranslateQueryPassesACancellableContext(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		_, err := translateWithTimeout(hangingProvider{}, "m", "sys", "q", 50*time.Millisecond)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want the context deadline surfaced, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("translateWith ignored its deadline — a stalled provider would hang the app forever")
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
