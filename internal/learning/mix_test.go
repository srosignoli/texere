package learning

import "testing"

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// A Collection that never set a Mix studies with the default one.
func TestLoadMixDefaultsWhenUnset(t *testing.T) {
	s := openStore(t)
	m, err := s.LoadMix()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Mix{Source: 20, Extract: 15, Card: 65}); m != want {
		t.Errorf("LoadMix() = %+v, want %+v", m, want)
	}
}

// The Mix belongs to the Collection: it is still there after the store is
// closed and opened again.
func TestSaveMixRoundTrips(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMix(Mix{Source: 5, Extract: 0, Card: 95}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	m, err := s2.LoadMix()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Mix{Source: 5, Extract: 0, Card: 95}); m != want {
		t.Errorf("LoadMix() = %+v, want %+v", m, want)
	}
}

// A Mix with every weight at 0 would serve nothing at all; it reads as the
// default instead.
func TestLoadMixAllZerosIsTheDefault(t *testing.T) {
	s := openStore(t)
	if err := s.SaveMix(Mix{}); err != nil {
		t.Fatal(err)
	}
	m, err := s.LoadMix()
	if err != nil {
		t.Fatal(err)
	}
	if m != DefaultMix {
		t.Errorf("LoadMix() = %+v, want the default %+v", m, DefaultMix)
	}
}

// A share cannot be negative. The rejected Mix must not replace the stored one.
func TestSaveMixRejectsNegatives(t *testing.T) {
	s := openStore(t)
	if err := s.SaveMix(Mix{Source: 10, Extract: 10, Card: 80}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMix(Mix{Source: -1, Extract: 10, Card: 80}); err == nil {
		t.Fatal("SaveMix accepted a negative weight")
	}
	m, err := s.LoadMix()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Mix{Source: 10, Extract: 10, Card: 80}); m != want {
		t.Errorf("LoadMix() = %+v after a rejected save, want %+v", m, want)
	}
}
