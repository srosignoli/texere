package mediasearch

import (
	"path"
	"regexp"
	"strings"
)

// llPrefixRe matches the fixed head of a Lingua Libre filename:
// "LL-{Qcode} ({iso})-". Everything after it is "{speaker}-{word}", which
// cannot be split unambiguously because either part may contain a hyphen.
var llPrefixRe = regexp.MustCompile(`^LL-Q\d+ \(([a-z-]+)\)-`)

// matchesWord reports whether name is a recording of exactly word, by testing
// the trailing "-{word}" before the extension. This is what excludes results
// that matched a contributor's name (a speaker called "Nattes à chat" answering
// a search for "chat") and compounds (pie-eyed, pie chart) that the search
// ranks above the bare word. name must not carry the "File:" prefix.
func matchesWord(name, word string) bool {
	w := strings.TrimSpace(word)
	if w == "" {
		return false
	}
	base := strings.TrimSuffix(name, path.Ext(name))
	return strings.HasSuffix(strings.ToLower(base), "-"+strings.ToLower(w))
}

// parseLinguaLibre extracts the display fields from a Lingua Libre filename.
// The speaker is recoverable only when the word is already known — i.e. when
// matchesWord passes — so on a non-exact name no split is guessed: the speaker
// is empty and the label is the whole remainder. For a name that is not from
// Lingua Libre at all, lang and speaker are empty and the label is the name
// without its extension. name must not carry the "File:" prefix.
func parseLinguaLibre(name, word string) (lang, speaker, label string) {
	base := strings.TrimSuffix(name, path.Ext(name))
	m := llPrefixRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", base
	}
	lang = m[1]
	rest := base[len(m[0]):] // "{speaker}-{word}", ambiguous on its own
	if matchesWord(name, word) {
		w := strings.TrimSpace(word)
		speaker = strings.TrimSuffix(rest, "-"+rest[len(rest)-len(w):])
		return lang, speaker, rest[len(rest)-len(w):]
	}
	return lang, "", rest
}
