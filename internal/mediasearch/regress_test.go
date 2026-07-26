package mediasearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Openverse rejects page_size > 20 for anonymous requests with a 401 (not a
// 400), so an unclamped limit made every live search fail. Regression test.
func TestOpenverseClampsPageSizeForAnonymousRequests(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	if _, err := (Openverse{BaseURL: srv.URL}).Search(context.Background(), "apple", "image", 24); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(gotQuery, "page_size=20") {
		t.Errorf("query = %q, want page_size clamped to 20", gotQuery)
	}
}

// A non-2xx response's body carries the reason (Openverse's 401 says exactly
// why); reporting only the status code made this undiagnosable from the UI.
func TestGetJSONErrorIncludesResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"page_size may not exceed 20 for anonymous requests"}`))
	}))
	defer srv.Close()

	err := getJSON(context.Background(), srv.URL, &struct{}{})
	if err == nil {
		t.Fatal("expected an error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q should name the status", err)
	}
	if !strings.Contains(err.Error(), "page_size may not exceed 20") {
		t.Errorf("error %q should include the response body's reason", err)
	}
}
