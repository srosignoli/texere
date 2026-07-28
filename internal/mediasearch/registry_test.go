package mediasearch

import "testing"

func TestProvidersCoverAllThreeKinds(t *testing.T) {
	ps := Providers()
	if len(ps) != 7 {
		t.Fatalf("got %d providers, want 7", len(ps))
	}
	byID := map[string]Provider{}
	for _, p := range ps {
		if p.Name() == "" || len(p.Kinds()) == 0 {
			t.Errorf("provider %q is incompletely described", p.ID())
		}
		byID[p.ID()] = p
	}
	for _, id := range []string{
		"commons", "openverse", "pronunciation", "commons-audio",
		"commons-video", "peertube", "archive",
	} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing provider %q", id)
		}
	}
	// Openverse's audio endpoint indexes music, so it stays image-only.
	if k := byID["openverse"].Kinds(); len(k) != 1 || k[0] != "image" {
		t.Errorf("openverse Kinds = %v, want [image] only", k)
	}
	// Every video provider must be resolvable, or picking one cannot produce a
	// playable URL.
	for _, id := range []string{"commons-video", "peertube", "archive"} {
		if _, ok := byID[id].(VideoResolver); !ok {
			t.Errorf("provider %q does not implement VideoResolver", id)
		}
	}
}

func TestFindKnownAndUnknown(t *testing.T) {
	p, ok := Find("commons")
	if !ok || p.ID() != "commons" {
		t.Errorf("Find(commons) = %v, %v", p, ok)
	}
	if _, ok := Find("nope"); ok {
		t.Error("Find(nope) reported ok, want false")
	}
}
