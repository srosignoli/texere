package mediasearch

import (
	"regexp"
	"strings"
)

// Language is one selectable Lingua Libre language: a display name and the
// Wikidata item code its recordings are filed under (LL-Q1321 = Spanish).
type Language struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

// languages is ordered by measured Lingua Libre recording volume (Commons
// `intitle:LL-<code>` totalhits), so the best-covered languages appear first.
// Every code here was verified to return files titled `LL-<code> (<iso>)-…`.
//
// Deliberately absent: Chinese (Q7850, 3 recordings) and Norwegian Bokmål
// (Q25167, 16) — too sparse to be anything but a dead end in a dropdown.
// Indonesian is absent because Q9240's recordings are all Minangkabau (min),
// so it is listed under that name instead. Unlisted languages remain reachable
// by typing a raw Q-code.
var languages = []Language{
	{"French", "Q150"},          // 431895
	{"English", "Q1860"},        // 106904
	{"Polish", "Q809"},          // 97537
	{"Esperanto", "Q143"},       // 36348
	{"Russian", "Q7737"},        // 31802
	{"Ukrainian", "Q8798"},      // 27402
	{"German", "Q188"},          // 25915
	{"Romanian", "Q7913"},       // 24088
	{"Catalan", "Q7026"},        // 22920
	{"Basque", "Q8752"},         // 20686
	{"Spanish", "Q1321"},        // 19140
	{"Occitan", "Q14185"},       // 14188
	{"Arabic", "Q13955"},        // 13738
	{"Italian", "Q652"},         // 12235
	{"Swedish", "Q9027"},        // 9726
	{"Portuguese", "Q5146"},     // 9339
	{"Minangkabau", "Q9240"},    // 7017
	{"Turkish", "Q256"},         // 6705
	{"Czech", "Q9056"},          // 6232
	{"Hebrew", "Q9288"},         // 3615
	{"Breton", "Q12107"},        // 3451
	{"Hindi", "Q1568"},          // 3430
	{"Vietnamese", "Q9199"},     // 3100
	{"Dutch", "Q7411"},          // 1971
	{"Japanese", "Q5287"},       // 1045
	{"Korean", "Q9176"},         // 1026
	{"Greek", "Q9129"},          // 360
	{"Thai", "Q9217"},           // 289
	{"Hungarian", "Q9067"},      // 259
	{"Finnish", "Q1412"},        // 220
	{"Danish", "Q9035"},         // 147
}

// Languages returns the selectable languages in display order.
func Languages() []Language { return languages }

var qCodeRe = regexp.MustCompile(`^Q\d+$`)

// ResolveLanguage maps a display name to its Wikidata code. A value that is
// already a raw Q-code passes through unchanged, so a language missing from the
// curated list is still reachable.
func ResolveLanguage(nameOrCode string) (string, bool) {
	s := strings.TrimSpace(nameOrCode)
	if s == "" {
		return "", false
	}
	if qCodeRe.MatchString(s) {
		return s, true
	}
	for _, l := range languages {
		if strings.EqualFold(l.Name, s) {
			return l.Code, true
		}
	}
	return "", false
}
