package mediasearch

import "testing"

func TestProvidersIncludesBothAndAllServeImages(t *testing.T) {
	ps := Providers()
	if len(ps) != 2 {
		t.Fatalf("got %d providers, want 2", len(ps))
	}
	seen := map[string]bool{}
	for _, p := range ps {
		seen[p.ID()] = true
		if p.Name() == "" {
			t.Errorf("provider %q has an empty Name", p.ID())
		}
		if len(p.Kinds()) == 0 || p.Kinds()[0] != "image" {
			t.Errorf("provider %q Kinds = %v, want image first", p.ID(), p.Kinds())
		}
	}
	if !seen["commons"] || !seen["openverse"] {
		t.Errorf("providers = %v, want commons and openverse", seen)
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
