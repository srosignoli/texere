package mediasearch

import "testing"

func TestProvidersCoverBothKinds(t *testing.T) {
	ps := Providers()
	if len(ps) != 4 {
		t.Fatalf("got %d providers, want 4", len(ps))
	}
	byID := map[string]Provider{}
	for _, p := range ps {
		if p.Name() == "" || len(p.Kinds()) == 0 {
			t.Errorf("provider %q is incompletely described", p.ID())
		}
		byID[p.ID()] = p
	}
	for _, id := range []string{"commons", "openverse", "pronunciation", "commons-audio"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing provider %q", id)
		}
	}
	// Openverse's audio endpoint indexes music, so it stays image-only.
	if k := byID["openverse"].Kinds(); len(k) != 1 || k[0] != "image" {
		t.Errorf("openverse Kinds = %v, want [image] only", k)
	}
	if k := byID["pronunciation"].Kinds(); k[0] != "audio" {
		t.Errorf("pronunciation Kinds = %v, want audio", k)
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
