package stt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscribeJSON(t *testing.T) {
	var gotAuth, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = r.ParseMultipartForm(1 << 20)
		gotModel = r.FormValue("model")
		if f, _, err := r.FormFile("file"); err == nil {
			f.Close()
		}
		w.Write([]byte(`{"text":"hello world"}`))
	}))
	defer srv.Close()
	got, err := Transcribe(context.Background(), srv.URL, "sk-x", "whisper-1", []byte("AUDIO"), "a.wav")
	if err != nil || got != "hello world" {
		t.Fatalf("got %q err %v", got, err)
	}
	if gotAuth != "Bearer sk-x" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotModel != "whisper-1" {
		t.Errorf("model = %q", gotModel)
	}
}

func TestTranscribePlainTextFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("just plain text"))
	}))
	defer srv.Close()
	got, err := Transcribe(context.Background(), srv.URL, "", "m", []byte("A"), "a.webm")
	if err != nil || got != "just plain text" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestTranscribeNoKeyNoAuthHeader(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.Write([]byte(`{"text":"x"}`))
	}))
	defer srv.Close()
	_, _ = Transcribe(context.Background(), srv.URL, "", "m", []byte("A"), "a.wav")
	if hadAuth {
		t.Error("no Authorization header expected when key is empty")
	}
}

func TestTranscribeErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("bad model"))
	}))
	defer srv.Close()
	if _, err := Transcribe(context.Background(), srv.URL, "", "m", []byte("A"), "a.wav"); err == nil {
		t.Fatal("expected error on 400")
	}
}
