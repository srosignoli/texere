package mediasearch

import "testing"

func TestLanguagesAreOrderedAndWellFormed(t *testing.T) {
	ls := Languages()
	if len(ls) < 25 {
		t.Fatalf("got %d languages, want at least 25", len(ls))
	}
	// French has by far the most Lingua Libre recordings and must lead, so the
	// best-covered languages are the ones a user sees first.
	if ls[0].Name != "French" || ls[0].Code != "Q150" {
		t.Errorf("first language = %+v, want French/Q150", ls[0])
	}
	seen := map[string]bool{}
	for _, l := range ls {
		if l.Name == "" || l.Code == "" {
			t.Errorf("incomplete entry: %+v", l)
		}
		if l.Code[0] != 'Q' {
			t.Errorf("code %q should be a Wikidata Q-code", l.Code)
		}
		if seen[l.Code] {
			t.Errorf("duplicate code %q", l.Code)
		}
		seen[l.Code] = true
	}
	// Verified as having 3 and 16 recordings respectively — a dropdown entry
	// that returns nothing is worse than its absence.
	if seen["Q7850"] || seen["Q25167"] {
		t.Error("Q7850 (Chinese) and Q25167 (Norwegian) are too sparse to list")
	}
}

func TestResolveLanguageByNameAndRawCode(t *testing.T) {
	if got, ok := ResolveLanguage("Spanish"); !ok || got != "Q1321" {
		t.Errorf("ResolveLanguage(Spanish) = %q, %v; want Q1321, true", got, ok)
	}
	// A raw Q-code passes through, so an unlisted language stays reachable.
	if got, ok := ResolveLanguage("Q12345"); !ok || got != "Q12345" {
		t.Errorf("ResolveLanguage(Q12345) = %q, %v; want passthrough", got, ok)
	}
	if _, ok := ResolveLanguage("Klingon"); ok {
		t.Error("ResolveLanguage(Klingon) reported ok, want false")
	}
	if _, ok := ResolveLanguage(""); ok {
		t.Error("ResolveLanguage(empty) reported ok, want false")
	}
}
