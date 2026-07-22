package main

import (
	"strings"
	"testing"
)

func TestNewStreamID(t *testing.T) {
	a, _ := newStreamID(), newStreamID()
	_ = a
	x, y := newStreamID(), newStreamID()
	if x == y {
		t.Fatalf("ids should differ: %q", x)
	}
	if !strings.HasPrefix(x, "llm-") {
		t.Fatalf("unexpected id: %q", x)
	}
}
