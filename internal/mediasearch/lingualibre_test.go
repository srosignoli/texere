package mediasearch

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMatchesWord(t *testing.T) {
	cases := []struct {
		name string
		word string
		want bool
	}{
		// Real result for the query "manzana".
		{"LL-Q1321 (spa)-Rdrg109-manzana.wav", "manzana", true},
		// Real collision: the speaker is "Nattes à chat", the word is "femme".
		{"LL-Q150 (fra)-Nattes à chat-femme.wav", "chat", false},
		// Real compounds that outrank the bare word for the query "pie".
		{"LL-Q1860 (eng)-I learned some phrases-pie-eyed.wav", "pie", false},
		{"LL-Q1860 (eng)-Neøn-pie chart.wav", "pie", false},
		{"LL-Q1860 (eng)-ManjiroLL-pie.wav", "pie", true},
		// Inflected forms are different lemmas and must not match.
		{"LL-Q188 (deu)-Sebastian Wallroth-Äpfel.wav", "Apfel", false},
		{"LL-Q188 (deu)-Sebastian Wallroth-Apfel.wav", "Apfel", true},
		// Case-insensitive, and surrounding whitespace in the query is ignored.
		{"LL-Q188 (deu)-Sebastian Wallroth-Apfel.wav", "apfel", true},
		{"LL-Q188 (deu)-Sebastian Wallroth-Apfel.wav", "  Apfel  ", true},
		// A multi-word query matches when it is the trailing segment.
		{"LL-Q1860 (eng)-Neøn-pie chart.wav", "pie chart", true},
	}
	for _, c := range cases {
		if got := matchesWord(c.name, c.word); got != c.want {
			t.Errorf("matchesWord(%q, %q) = %v, want %v", c.name, c.word, got, c.want)
		}
	}
}

func TestParseLinguaLibreExactMatch(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q1321 (spa)-Rdrg109-manzana.wav", "manzana")
	if lang != "spa" || speaker != "Rdrg109" || label != "manzana" {
		t.Errorf("got (%q, %q, %q), want (spa, Rdrg109, manzana)", lang, speaker, label)
	}
}

func TestParseLinguaLibreSpeakerContainingHyphen(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q188 (deu)-Sebastian-Wallroth-Apfel.wav", "Apfel")
	if lang != "deu" || speaker != "Sebastian-Wallroth" || label != "Apfel" {
		t.Errorf("got (%q, %q, %q), want (deu, Sebastian-Wallroth, Apfel)", lang, speaker, label)
	}
}

// The word itself contains a hyphen AND the speaker contains spaces. This is
// the case that distinguishes the real implementation from a naive split on the
// last hyphen, which would yield speaker="I learned some phrases-pie",
// label="eyed". It is a real filename returned by a search for "pie".
func TestParseLinguaLibreWordContainingHyphen(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q1860 (eng)-I learned some phrases-pie-eyed.wav", "pie-eyed")
	if lang != "eng" || speaker != "I learned some phrases" || label != "pie-eyed" {
		t.Errorf("got (%q, %q, %q), want (eng, I learned some phrases, pie-eyed)", lang, speaker, label)
	}
}

func TestParseLinguaLibreWordContainingSpace(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q1860 (eng)-Neøn-pie chart.wav", "pie chart")
	if lang != "eng" || speaker != "Neøn" || label != "pie chart" {
		t.Errorf("got (%q, %q, %q), want (eng, Neøn, pie chart)", lang, speaker, label)
	}
}

// On a non-exact match (the unfiltered fallback) the speaker/word split is
// genuinely unknowable, so no guess is made: the whole remainder is the label.
func TestParseLinguaLibreNonExactMakesNoGuess(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q150 (fra)-Nattes à chat-femme.wav", "chat")
	if lang != "fra" {
		t.Errorf("lang = %q, want fra", lang)
	}
	if speaker != "" {
		t.Errorf("speaker = %q, want empty (split is unknowable)", speaker)
	}
	if label != "Nattes à chat-femme" {
		t.Errorf("label = %q, want the full remainder", label)
	}
}

// Real crash: the matched suffix ("Q1321 (spa)-Rdrg109-manzana") reaches back
// into the "LL-Q…(iso)-" prefix that matchesWord tested against but that
// parseLinguaLibre's `rest` already excludes, so len(rest) < len(word). The
// naive `rest[len(rest)-len(w):]` slice went negative and panicked. The fix
// scans for the split instead of trusting a byte length, so this must now
// return cleanly: no valid split exists, so it falls back to the honest
// no-guess case rather than crashing.
func TestParseLinguaLibreQueryReachesIntoPrefixDoesNotPanic(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q1321 (spa)-Rdrg109-manzana.wav", "Q1321 (spa)-Rdrg109-manzana")
	if lang != "spa" {
		t.Errorf("lang = %q, want spa", lang)
	}
	if speaker != "" {
		t.Errorf("speaker = %q, want empty (no valid split found)", speaker)
	}
	if label != "Rdrg109-manzana" {
		t.Errorf("label = %q, want the full remainder", label)
	}
}

// Real corruption: case folding can change a query's UTF-8 byte length
// (Turkish İ is 2 bytes, its folded "i" is 1), so slicing by len(word) can
// land mid-rune. strings.ToLower folds İ to plain "i" (so matchesWord's
// suffix test passes) but strings.EqualFold does not consider İ and i
// equivalent, so the split scan finds no matching hyphen and must fall back
// to the no-guess case — never a corrupt label, and never a speaker that
// swallows the word.
func TestParseLinguaLibreTurkishFoldingNeverCorruptsUTF8(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("LL-Q256 (tur)-Speaker-İstanbul.wav", "istanbul")
	if lang != "tur" {
		t.Errorf("lang = %q, want tur", lang)
	}
	if !utf8.ValidString(speaker) {
		t.Errorf("speaker = %q is not valid UTF-8", speaker)
	}
	if !utf8.ValidString(label) {
		t.Errorf("label = %q is not valid UTF-8", label)
	}
	if strings.Contains(strings.ToLower(speaker), "istanbul") {
		t.Errorf("speaker = %q must not contain the word", speaker)
	}
}

func TestParseLinguaLibreNonLinguaLibreName(t *testing.T) {
	lang, speaker, label := parseLinguaLibre("Tango de Manzana (ISRC USUAN1100404).mp3", "manzana")
	if lang != "" || speaker != "" {
		t.Errorf("got (%q, %q), want empties for a non-Lingua-Libre name", lang, speaker)
	}
	if label != "Tango de Manzana (ISRC USUAN1100404)" {
		t.Errorf("label = %q, want the name without its extension", label)
	}
}
