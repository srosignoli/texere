package search_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"texere/internal/content"
	"texere/internal/graph"
	"texere/internal/search"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func testFixturesDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

func testRegistry() *content.SSTRegistry {
	reg := &content.SSTRegistry{
		Arrows:      make(map[string]content.ArrowDef),
		Annotations: make(map[string]content.AnnotDef),
	}
	pairs := [][2]string{
		{"hp", "ph"}, // hanzi-pinyin / pinyin-hanzi
		{"eh", "he"}, // english-hanzi / hanzi-english
	}
	for _, p := range pairs {
		reg.Arrows[p[0]] = content.ArrowDef{Code: p[0], LongName: p[0], Reverse: p[1], MetaType: "express"}
		reg.Arrows[p[1]] = content.ArrowDef{Code: p[1], LongName: p[1], Reverse: p[0], MetaType: "express"}
	}
	reg.Arrows["means"] = content.ArrowDef{Code: "means", LongName: "means", MetaType: "express"}
	reg.Arrows["next"] = content.ArrowDef{Code: "next", LongName: "next", MetaType: "leadsto"}
	reg.Arrows["leadsto"] = content.ArrowDef{Code: "leadsto", LongName: "leads to", MetaType: "leadsto"}
	// doors.n4l uses the leadsto `fwd`/`bwd` pair (mirrors the real SSTconfig
	// `+ leads to (fwd) - comes from (bwd)`), so the causal cone follows it. The
	// long names must stay distinct from the "leadsto" arrow above — a duplicate
	// LongName collides in InsertArrowDirectory and corrupts both arrows' links.
	reg.Arrows["fwd"] = content.ArrowDef{Code: "fwd", LongName: "forward to", Reverse: "bwd", MetaType: "leadsto"}
	reg.Arrows["bwd"] = content.ArrowDef{Code: "bwd", LongName: "backward from", Reverse: "fwd", MetaType: "leadsto"}
	reg.Arrows["freq"] = content.ArrowDef{Code: "freq", LongName: "has frequency", MetaType: "properties"}
	reg.Arrows["state"] = content.ArrowDef{Code: "state", LongName: "has state", MetaType: "properties"}
	reg.Arrows["pt-of"] = content.ArrowDef{Code: "pt-of", LongName: "is part of", MetaType: "contains"}
	return reg
}

// buildTestGraph builds the fixture graph in a temp dir and returns a ready SearchService.
func buildTestGraph(t *testing.T) *search.SearchService {
	svc, _ := buildTestGraphInDir(t)
	return svc
}

// buildTestGraphInDir is like buildTestGraph but also returns the collection
// directory so tests can manipulate the underlying sst_data/ or bleve/ paths
// (e.g. simulate a corrupt index for T059).
func buildTestGraphInDir(t *testing.T) (*search.SearchService, string) {
	t.Helper()
	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(testFixturesDir(t), "chinese.n4l"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chinese.n4l"), data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)

	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)
	return svc, dir
}

func nodeTexts(nodes []search.GraphNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.NodeText
	}
	return out
}

func listingTitles(items []search.ListingItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

func hasOrbitNeighbor(entries []search.OrbitEntry, text string) bool {
	for _, e := range entries {
		if e.NeighborText == text {
			return true
		}
	}
	return false
}

func normalizeSearch(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ── node search ───────────────────────────────────────────────────────────────

func TestSearch_PlainText(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search("fangzi")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.GraphUnavailable {
		t.Fatal("graph unavailable")
	}
	if len(res.Nodes) == 0 {
		t.Fatal("expected nodes for 'fangzi'")
	}
}

func TestSearch_AccentInsensitive(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search("fangzi")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, n := range res.Nodes {
		// The stored node uses diacritics (fángzi); query without diacritics must match.
		if strings.Contains(n.NodeText, "z") { // rough check: "fángzi" contains z
			found = true
		}
	}
	if !found {
		t.Errorf("accent-insensitive search failed; got %v", nodeTexts(res.Nodes))
	}
}

func TestSearch_ChapterFilter_Positive(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search(`fangzi \chapter chinese`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, n := range res.Nodes {
		if strings.Contains(strings.ToLower(n.NodeText), "fangzi") ||
			strings.Contains(n.NodeText, "z") {
			found = true
		}
	}
	if !found {
		t.Errorf("fangzi not found with \\chapter chinese; got %v", nodeTexts(res.Nodes))
	}
}

func TestSearch_ChapterFilter_Negative(t *testing.T) {
	svc := buildTestGraph(t)

	// nǐ hǎo is in Greetings, not Chinese; should not appear with \chapter chinese.
	res, err := svc.Search(`ni hao \chapter chinese`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, n := range res.Nodes {
		norm := strings.ToLower(n.NodeText)
		if strings.Contains(norm, "ni") && strings.Contains(norm, "hao") {
			t.Errorf("node from wrong chapter returned: %q", n.NodeText)
		}
	}
}

// ── orbit ─────────────────────────────────────────────────────────────────────

func TestSearch_OrbitPresent(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search("fangzi")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatal("no nodes returned")
	}

	var fangziNode *search.GraphNode
	for i := range res.Nodes {
		if strings.Contains(strings.ToLower(res.Nodes[i].NodeText), "fangzi") ||
			strings.Contains(res.Nodes[i].NodeText, "z") {
			fangziNode = &res.Nodes[i]
			break
		}
	}
	if fangziNode == nil {
		t.Fatalf("fangzi node not found; got %v", nodeTexts(res.Nodes))
	}
	if len(fangziNode.Orbit) == 0 {
		t.Errorf("expected orbit entries for fangzi, got none")
	}
	if !hasOrbitNeighbor(fangziNode.Orbit, "房子") {
		t.Errorf("expected 房子 in orbit; got %+v", fangziNode.Orbit)
	}
}

func TestSearch_OrbitAfterSimulatedRestart(t *testing.T) {
	dir := t.TempDir()
	data, _ := os.ReadFile(filepath.Join(testFixturesDir(t), "chinese.n4l"))
	_ = os.WriteFile(filepath.Join(dir, "chinese.n4l"), data, 0644)
	reg := testRegistry()

	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("build: %v", err)
	}

	// Simulate app restart: wipe global in-memory state.
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)

	res, err := svc.Search("fangzi")
	if err != nil {
		t.Fatalf("Search after restart: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatal("no nodes after restart")
	}
	var fangziNode *search.GraphNode
	for i := range res.Nodes {
		if strings.Contains(strings.ToLower(res.Nodes[i].NodeText), "fangzi") ||
			strings.Contains(res.Nodes[i].NodeText, "z") {
			fangziNode = &res.Nodes[i]
			break
		}
	}
	if fangziNode == nil {
		t.Fatalf("fangzi not found after restart; got %v", nodeTexts(res.Nodes))
	}
	if len(fangziNode.Orbit) == 0 {
		t.Errorf("orbit empty after simulated restart — rebuildMemoryLinks failed")
	}
}

// ── chapter listing ───────────────────────────────────────────────────────────

func TestChapterListing_All(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search(`\chapter any`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	titles := listingTitles(res.Listings)
	sort.Strings(titles)

	for _, want := range []string{"Chinese", "Greetings"} {
		found := false
		for _, got := range titles {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected chapter %q; got %v", want, titles)
		}
	}
}

func TestChapterListing_FilterCaseInsensitive(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search(`\chapter chinese`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Listings) == 0 {
		t.Fatal("expected chapters for \\chapter chinese")
	}
	for _, item := range res.Listings {
		if !strings.Contains(strings.ToLower(item.Title), "chinese") {
			t.Errorf("unexpected chapter %q for filter 'chinese'", item.Title)
		}
	}
}

// TestChapterListing_NodesAlwaysEmpty preserves the original NoNodeList
// invariant: the chapter listing path must not populate ListingItem.Nodes
// (that field is reserved for callers that genuinely emit node texts). The
// new TOC behavior surfaces contexts via Contexts, not Nodes.
func TestChapterListing_NodesAlwaysEmpty(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search(`\chapter any`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, item := range res.Listings {
		if item.Kind == "chapter" && len(item.Nodes) > 0 {
			t.Errorf("chapter listing item %q should not contain Nodes (got %v)", item.Title, item.Nodes)
		}
	}
}

// TestChapterListing_SpecificChapterListsContexts verifies that
// `\chapter <name>` populates Contexts with the distinct :: ctx :: headers
// authored under the chapter, in stable (alphabetical) order, with no
// empty-state Detail when contexts exist.
func TestChapterListing_SpecificChapterListsContexts(t *testing.T) {
	dir := t.TempDir()
	body := "" +
		"- Tools\n\n" +
		":: alpha ::\n\n" +
		"widget\n\"  (uses) thingA\n\n" +
		":: beta ::\n\n" +
		"widget\n\"  (runs) thingB\n"
	if err := os.WriteFile(filepath.Join(dir, "tools.n4l"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	reg := &content.SSTRegistry{
		Arrows:      make(map[string]content.ArrowDef),
		Annotations: make(map[string]content.AnnotDef),
	}
	reg.Arrows["uses"] = content.ArrowDef{Code: "uses", LongName: "uses", MetaType: "express"}
	reg.Arrows["runs"] = content.ArrowDef{Code: "runs", LongName: "runs", MetaType: "express"}

	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatal(err)
	}

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)

	res, err := svc.Search(`\chapter Tools`)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Listings) != 1 {
		t.Fatalf("want 1 listing, got %d", len(res.Listings))
	}
	item := res.Listings[0]
	if item.Title != "Tools" {
		t.Errorf("want Title=Tools, got %q", item.Title)
	}
	wantCtx := []string{"alpha", "beta"}
	if !reflect.DeepEqual(item.Contexts, wantCtx) {
		t.Errorf("want Contexts=%v, got %v", wantCtx, item.Contexts)
	}
	if item.Detail != "" {
		t.Errorf("want empty Detail when contexts exist, got %q", item.Detail)
	}
}

// TestChapterListing_SpecificChapterEmptyContextsHint verifies the empty-state
// hint: when a specific chapter exists but carries no contexts, Detail must
// guide the user toward \notes.
func TestChapterListing_SpecificChapterEmptyContextsHint(t *testing.T) {
	svc := buildTestGraph(t)

	// chinese.n4l fixture: chapters "Chinese" and "Greetings" both have zero
	// :: ctx :: headers, so this is the canonical empty-state case.
	res, err := svc.Search(`\chapter Chinese`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var found *search.ListingItem
	for i := range res.Listings {
		if res.Listings[i].Title == "Chinese" {
			found = &res.Listings[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("chapter \"Chinese\" missing from listings: %+v", res.Listings)
	}
	if len(found.Contexts) != 0 {
		t.Errorf("want empty Contexts for chapter \"Chinese\", got %v", found.Contexts)
	}
	wantSubstr := "no contexts defined for chapter Chinese"
	if !strings.Contains(found.Detail, wantSubstr) {
		t.Errorf("Detail missing hint %q: got %q", wantSubstr, found.Detail)
	}
	if !strings.Contains(found.Detail, `\notes Chinese`) {
		t.Errorf("Detail missing \\notes redirect: got %q", found.Detail)
	}
}

// TestChapterListing_WildcardIncludesContexts verifies that `\chapter any`
// populates Contexts on each row (when contexts exist) but does NOT raise
// the empty-state Detail hint — wildcard listings should enumerate quietly.
func TestChapterListing_WildcardIncludesContexts(t *testing.T) {
	dir := t.TempDir()
	body := "" +
		"- ChapA\n\n" +
		":: ctxA1 ::\n\n" +
		"alphaNode\n\"  (uses) target1\n\n" +
		"- ChapB\n\n" +
		"betaNode\n\"  (uses) target2\n"
	if err := os.WriteFile(filepath.Join(dir, "wild.n4l"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	reg := &content.SSTRegistry{
		Arrows:      make(map[string]content.ArrowDef),
		Annotations: make(map[string]content.AnnotDef),
	}
	reg.Arrows["uses"] = content.ArrowDef{Code: "uses", LongName: "uses", MetaType: "express"}

	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatal(err)
	}

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)

	res, err := svc.Search(`\chapter any`)
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]search.ListingItem{}
	for _, l := range res.Listings {
		if l.Kind == "chapter" {
			byTitle[l.Title] = l
		}
	}
	a, ok := byTitle["ChapA"]
	if !ok {
		t.Fatalf("ChapA missing from listings: %+v", res.Listings)
	}
	if !reflect.DeepEqual(a.Contexts, []string{"ctxA1"}) {
		t.Errorf("ChapA: want Contexts=[ctxA1], got %v", a.Contexts)
	}
	if a.Detail != "" {
		t.Errorf("ChapA: wildcard listing must not raise Detail hint, got %q", a.Detail)
	}
	b, ok := byTitle["ChapB"]
	if !ok {
		t.Fatalf("ChapB missing from listings: %+v", res.Listings)
	}
	if len(b.Contexts) != 0 {
		t.Errorf("ChapB: want empty Contexts, got %v", b.Contexts)
	}
	if b.Detail != "" {
		t.Errorf("ChapB: wildcard listing must not raise Detail hint, got %q", b.Detail)
	}
}

// ── exact match (!word! / |word|) ────────────────────────────────────────────

func TestSearch_ExactMatch_Hit(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search("!house!")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatal("expected 'house' node for exact query !house!")
	}
	for _, n := range res.Nodes {
		if strings.ToLower(strings.TrimSpace(n.NodeText)) != "house" {
			t.Errorf("exact query !house! returned unexpected node %q", n.NodeText)
		}
	}
}

func TestSearch_ExactMatch_Miss(t *testing.T) {
	svc := buildTestGraph(t)

	// "dwelling" is not a standalone node (only "a house, dwelling" exists).
	res, err := svc.Search("!dwelling!")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Nodes) != 0 {
		t.Errorf("exact query !dwelling! should return no results; got %v", nodeTexts(res.Nodes))
	}
}

func TestSearch_ExactMatch_PipeSyntax(t *testing.T) {
	svc := buildTestGraph(t)

	r1, err := svc.Search("!house!")
	if err != nil {
		t.Fatalf("Search !house!: %v", err)
	}
	r2, err := svc.Search("|house|")
	if err != nil {
		t.Fatalf("Search |house|: %v", err)
	}
	if len(r1.Nodes) != len(r2.Nodes) {
		t.Errorf("|house| (%d) and !house! (%d) returned different result counts",
			len(r2.Nodes), len(r1.Nodes))
	}
}

func TestSearch_ExactMatch_AccentInsensitive(t *testing.T) {
	svc := buildTestGraph(t)

	// |fangzi| (no diacritics) must match the stored node "fángzi".
	res, err := svc.Search("|fangzi|")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, n := range res.Nodes {
		if strings.Contains(n.NodeText, "ngzi") { // "fángzi" contains "ngzi"
			found = true
		}
	}
	if !found {
		t.Errorf("|fangzi| should match 'fángzi'; got %v", nodeTexts(res.Nodes))
	}
}

func TestSearch_ExactMatch_WithChapter(t *testing.T) {
	svc := buildTestGraph(t)

	res, err := svc.Search(`!house! \chapter chinese`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatal("expected 'house' in Chinese chapter for !house! \\chapter chinese")
	}
	for _, n := range res.Nodes {
		if strings.ToLower(strings.TrimSpace(n.NodeText)) != "house" {
			t.Errorf("!house! \\chapter chinese returned unexpected node %q", n.NodeText)
		}
	}
}

// ── OR multi-term search ──────────────────────────────────────────────────────

func TestSearch_OR_MultiTerm(t *testing.T) {
	svc := buildTestGraph(t)

	// "house" and "wǒ" are distinct nodes. Query "house wo" (space = OR) must
	// return both. With phrase semantics, no node contains "house wo" → zero results.
	res, err := svc.Search("house wo")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	foundHouse, foundWo := false, false
	for _, n := range res.Nodes {
		norm := normalizeSearch(n.NodeText)
		if norm == "house" {
			foundHouse = true
		}
		if norm == "wǒ" || norm == "wo" { // stored as "wǒ" with diacritic
			foundWo = true
		}
	}
	if !foundHouse {
		t.Errorf("OR: expected 'house' node in results; got %v", nodeTexts(res.Nodes))
	}
	if !foundWo {
		t.Errorf("OR: expected 'wǒ' node in results; got %v", nodeTexts(res.Nodes))
	}
}

// ── context filter ────────────────────────────────────────────────────────────

func buildCookingGraph(t *testing.T) *search.SearchService {
	t.Helper()
	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(testFixturesDir(t), "cooking.n4l"))
	if err != nil {
		t.Fatalf("read cooking fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cooking.n4l"), data, 0644); err != nil {
		t.Fatalf("write cooking fixture: %v", err)
	}

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)

	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)
	return svc
}

func TestSearch_ContextFilter_Hit(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search(`fish \context recipes`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Fatal("expected 'fish soup' for fish \\context recipes; got no results")
	}
	found := false
	for _, n := range res.Nodes {
		if strings.Contains(strings.ToLower(n.NodeText), "fish") {
			found = true
		}
	}
	if !found {
		t.Errorf("context hit: expected fish node; got %v", nodeTexts(res.Nodes))
	}
}

func TestSearch_ContextFilter_Miss(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search(`fish \context desserts`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Nodes) != 0 {
		t.Errorf("context miss: expected no results for fish \\context desserts; got %v", nodeTexts(res.Nodes))
	}
}

func TestSearch_ContextFilter_NoFilter(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search("fish")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, n := range res.Nodes {
		if strings.Contains(strings.ToLower(n.NodeText), "fish") {
			found = true
		}
	}
	if !found {
		t.Errorf("no-filter: expected fish node without \\context filter; got %v", nodeTexts(res.Nodes))
	}
}

func TestSearch_ContextTags_Populated(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search("fish soup")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var fishNode *search.GraphNode
	for i := range res.Nodes {
		if strings.ToLower(res.Nodes[i].NodeText) == "fish soup" {
			fishNode = &res.Nodes[i]
			break
		}
	}
	if fishNode == nil {
		t.Fatalf("fish soup node not found; got %v", nodeTexts(res.Nodes))
	}
	found := false
	for _, tag := range fishNode.ContextTags {
		if tag == "recipes" {
			found = true
		}
	}
	if !found {
		t.Errorf("ContextTags: expected 'recipes' tag on fish soup; got %v", fishNode.ContextTags)
	}
}

// ── notes ─────────────────────────────────────────────────────────────────────

// noteTexts flattens every block's NodeText across all FullNotes so tests that
// care about membership but not structure can reuse the same helper.
func noteTexts(notes []search.FullNote) []string {
	out := []string{}
	for _, n := range notes {
		for _, b := range n.Blocks {
			out = append(out, b.NodeText)
		}
	}
	return out
}

func hasNoteBlock(notes []search.FullNote, nodeText string) bool {
	for _, n := range notes {
		for _, b := range n.Blocks {
			if b.NodeText == nodeText {
				return true
			}
		}
	}
	return false
}

func findNoteBlock(notes []search.FullNote, nodeText string) *search.NoteBlock {
	for ni := range notes {
		for bi := range notes[ni].Blocks {
			if notes[ni].Blocks[bi].NodeText == nodeText {
				return &notes[ni].Blocks[bi]
			}
		}
	}
	return nil
}

func TestNotes_ByChapter(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search(`\notes Cooking`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Notes) != 1 {
		t.Fatalf("\\notes Cooking: expected exactly 1 FullNote; got %d", len(res.Notes))
	}
	if res.Notes[0].Chapter != "Cooking" {
		t.Errorf("\\notes Cooking: FullNote.Chapter = %q; want %q", res.Notes[0].Chapter, "Cooking")
	}
	for _, want := range []string{"fish soup", "grilled salmon", "chocolate cake", "vanilla ice cream", "apple"} {
		if !hasNoteBlock(res.Notes, want) {
			t.Errorf("\\notes Cooking: expected block %q; got %v", want, noteTexts(res.Notes))
		}
	}
}

func TestNotes_ArrowResolution(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search(`\notes Cooking`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	fish := findNoteBlock(res.Notes, "fish soup")
	if fish == nil {
		t.Fatalf("fish soup block missing; got %v", noteTexts(res.Notes))
	}
	if len(fish.Arrows) != 1 {
		t.Fatalf("fish soup: expected 1 arrow (ditto means); got %d", len(fish.Arrows))
	}
	if fish.Arrows[0].ArrowCode != "means" {
		t.Errorf("fish soup arrow code: want %q got %q", "means", fish.Arrows[0].ArrowCode)
	}
	if fish.Arrows[0].Target != "a soup made with fish" {
		t.Errorf("fish soup arrow target: want %q got %q", "a soup made with fish", fish.Arrows[0].Target)
	}
	if fish.Arrows[0].ArrowLong == "" {
		t.Errorf("fish soup ArrowLong must be populated; got empty")
	}
}

func TestNotes_ByChapter_CaseInsensitive(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search(`\notes cooking`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Notes) == 0 {
		t.Fatal("\\notes cooking (lowercase): expected results, got none")
	}
	if !hasNoteBlock(res.Notes, "fish soup") {
		t.Errorf("\\notes cooking: expected 'fish soup'; got %v", noteTexts(res.Notes))
	}
}

func TestNotes_ByContext(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search(`\notes \context recipes`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, want := range []string{"fish soup", "grilled salmon"} {
		if !hasNoteBlock(res.Notes, want) {
			t.Errorf("\\notes \\context recipes: expected %q; got %v", want, noteTexts(res.Notes))
		}
	}
	for _, unwanted := range []string{"chocolate cake", "apple"} {
		if hasNoteBlock(res.Notes, unwanted) {
			t.Errorf("\\notes \\context recipes: unexpected %q; got %v", unwanted, noteTexts(res.Notes))
		}
	}
}

func TestNotes_ByChapterAndContext(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search(`\notes Cooking \context desserts`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !hasNoteBlock(res.Notes, "chocolate cake") {
		t.Errorf("\\notes Cooking \\context desserts: expected 'chocolate cake'; got %v", noteTexts(res.Notes))
	}
	for _, unwanted := range []string{"fish soup", "grilled salmon", "apple"} {
		if hasNoteBlock(res.Notes, unwanted) {
			t.Errorf("\\notes Cooking \\context desserts: unexpected %q; got %v", unwanted, noteTexts(res.Notes))
		}
	}
}

func TestNotes_ByContext_NoArrows(t *testing.T) {
	svc := buildCookingGraph(t)

	// "vanilla ice cream" is under :: desserts but has no arrows — must still appear.
	res, err := svc.Search(`\notes \context desserts`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !hasNoteBlock(res.Notes, "vanilla ice cream") {
		t.Errorf(`\notes \context desserts: expected "vanilla ice cream" (no-arrow node); got %v`, noteTexts(res.Notes))
	}
	if hasNoteBlock(res.Notes, "apple") {
		t.Errorf(`\notes \context desserts: unexpected "apple" (no-context node); got %v`, noteTexts(res.Notes))
	}
}

func TestNotes_Miss(t *testing.T) {
	svc := buildCookingGraph(t)

	res, err := svc.Search(`\notes NonExistent`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Notes) != 0 {
		t.Errorf("\\notes NonExistent: expected no notes; got %v", noteTexts(res.Notes))
	}
}

// ── sequence ──────────────────────────────────────────────────────────────────

func buildSequenceGraph(t *testing.T) *search.SearchService {
	t.Helper()
	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(testFixturesDir(t), "sequence.n4l"))
	if err != nil {
		t.Fatalf("read sequence fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sequence.n4l"), data, 0644); err != nil {
		t.Fatalf("write sequence fixture: %v", err)
	}

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)

	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)
	return svc
}

func TestSequence_ByName(t *testing.T) {
	svc := buildSequenceGraph(t)

	res, err := svc.Search(`\story step 1`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\story step 1: expected at least one chain; got none`)
	}
	chain := res.Structural[0]
	texts := make([]string, 0, len(chain.Nodes))
	for _, n := range chain.Nodes {
		texts = append(texts, n.NodeText)
	}
	for _, want := range []string{"step 1", "step 2", "step 3"} {
		found := false
		for _, txt := range texts {
			if strings.Contains(strings.ToLower(txt), want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("\\story step 1: expected %q in chain %v", want, texts)
		}
	}
}

func TestSequence_AnyInChapter(t *testing.T) {
	svc := buildSequenceGraph(t)

	res, err := svc.Search(`\story any \chapter Tutorial`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\story any \chapter Tutorial: expected at least one chain; got none`)
	}
}

func TestSequence_Miss(t *testing.T) {
	svc := buildSequenceGraph(t)

	res, err := svc.Search(`\story nonexistent_node_xyz`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) != 0 {
		t.Errorf(`\story nonexistent: expected empty; got %d chains`, len(res.Structural))
	}
}

// TestSequence_ImplicitViaContext guards the production fix: N4L blocks that
// use `:: _sequence_ ::` context instead of explicit `(next)` arrows must still
// be walkable via \story / \sequence. Also exercises context-label population
// so the frontend can group mixed sequences by their context.
func TestSequence_ImplicitViaContext(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join(testFixturesDir(t), "implicit_sequence.n4l"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "implicit_sequence.n4l"), data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)

	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}
	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)

	// Match a node common to the pod-lifecycle sequence.
	res, err := svc.Search(`\sequence "create a pod"`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\sequence "create a pod": expected at least one chain; got none`)
	}

	var found bool
	for _, chain := range res.Structural {
		if chain.Context != "pod lifecycle" {
			continue
		}
		found = true
		if len(chain.Nodes) < 3 {
			t.Errorf("expected pod-lifecycle chain to span 3+ steps; got %d", len(chain.Nodes))
		}
		if chain.Chapter != "Kubernetes" {
			t.Errorf("expected Chapter=Kubernetes on chain; got %q", chain.Chapter)
		}
	}
	if !found {
		var contexts []string
		for _, c := range res.Structural {
			contexts = append(contexts, c.Context)
		}
		t.Errorf(`expected a chain tagged Context="pod lifecycle"; got contexts=%v`, contexts)
	}

	// Wildcard query in the same chapter must return BOTH sequences as
	// separate chains — the pod-lifecycle and operator-upgrade blocks share
	// a chapter but have different `:: _sequence_, X ::` labels, so they must
	// not fuse into a single 7-step chain, and must not produce overlapping
	// suffix-chains (one chain per sequence, not one per node).
	resAll, err := svc.Search(`\seq any \chapter Kubernetes`)
	if err != nil {
		t.Fatalf(`\seq any \chapter Kubernetes: %v`, err)
	}
	ctxCount := make(map[string]int)
	for _, c := range resAll.Structural {
		ctxCount[c.Context]++
		if len(c.Nodes) >= 7 {
			t.Errorf("chain fused across context boundary: %d steps in context=%q",
				len(c.Nodes), c.Context)
		}
	}
	if ctxCount["pod lifecycle"] != 1 || ctxCount["operator upgrade"] != 1 {
		t.Errorf("expected exactly one chain per sequence block; got pod lifecycle=%d, operator upgrade=%d",
			ctxCount["pod lifecycle"], ctxCount["operator upgrade"])
	}
}

// ── paths (\from \to) ─────────────────────────────────────────────────────────

func buildPathsGraph(t *testing.T) *search.SearchService {
	t.Helper()
	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(testFixturesDir(t), "paths.n4l"))
	if err != nil {
		t.Fatalf("read paths fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "paths.n4l"), data, 0644); err != nil {
		t.Fatalf("write paths fixture: %v", err)
	}

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)

	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)
	return svc
}

func chainTexts(chain search.StructuralChain) []string {
	out := make([]string, 0, len(chain.Nodes))
	for _, n := range chain.Nodes {
		out = append(out, n.NodeText)
	}
	return out
}

func TestPath_ByName(t *testing.T) {
	svc := buildPathsGraph(t)

	res, err := svc.Search(`\from start \to goal`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\from start \to goal: expected at least one path; got none`)
	}
	for i, chain := range res.Structural {
		if chain.Kind != "path" {
			t.Errorf("chain %d: expected Kind=path; got %q", i, chain.Kind)
		}
		if chain.Chapter != "Maze" {
			t.Errorf("chain %d: expected Chapter=Maze; got %q", i, chain.Chapter)
		}
		if len(chain.Nodes) < 2 {
			t.Errorf("chain %d: expected >= 2 nodes; got %d", i, len(chain.Nodes))
			continue
		}
		// Every non-last node must carry arrow info for the outgoing edge.
		for j := 0; j < len(chain.Nodes)-1; j++ {
			if chain.Nodes[j].ArrowCode == "" || chain.Nodes[j].ArrowLong == "" {
				t.Errorf("chain %d step %d: missing arrow info (code=%q long=%q)",
					i, j, chain.Nodes[j].ArrowCode, chain.Nodes[j].ArrowLong)
			}
		}
		// Last node should not have a trailing arrow.
		last := chain.Nodes[len(chain.Nodes)-1]
		if last.ArrowCode != "" || last.ArrowLong != "" {
			t.Errorf("chain %d: last node should not have arrow; got code=%q long=%q",
				i, last.ArrowCode, last.ArrowLong)
		}
		texts := chainTexts(chain)
		if !strings.EqualFold(texts[0], "start") {
			t.Errorf("chain %d: expected first node 'start'; got %q (chain: %v)", i, texts[0], texts)
		}
		if !strings.EqualFold(texts[len(texts)-1], "goal") {
			t.Errorf("chain %d: expected last node 'goal'; got %q (chain: %v)", i, texts[len(texts)-1], texts)
		}
	}
	// At least one chain must exercise a mixed-arrow path (both "next" and "leadsto").
	var sawMixed bool
	for _, chain := range res.Structural {
		codes := make(map[string]bool)
		for _, n := range chain.Nodes {
			if n.ArrowCode != "" {
				codes[n.ArrowCode] = true
			}
		}
		if codes["next"] && codes["leadsto"] {
			sawMixed = true
			break
		}
	}
	if !sawMixed {
		// Not fatal — depends on which paths SST returns — but flag so a future
		// regression that drops mixed-arrow chains is visible.
		t.Logf(`no mixed-arrow path surfaced; found %d chain(s)`, len(res.Structural))
	}
}

func TestPath_WithDepth(t *testing.T) {
	svc := buildPathsGraph(t)

	// Guards that \depth N parses as Range (not as part of the node name).
	res, err := svc.Search(`\from start \to goal \depth 5`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\from start \to goal \depth 5: expected chains; got none`)
	}
	for _, chain := range res.Structural {
		if chain.Kind != "path" {
			t.Errorf("expected Kind=path; got %q", chain.Kind)
		}
	}
}

func TestPath_NoPath(t *testing.T) {
	svc := buildPathsGraph(t)

	res, err := svc.Search(`\from start \to unreachable_xyz`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) != 0 {
		t.Errorf("expected no chains for unreachable target; got %d", len(res.Structural))
	}
}

func TestPath_KindDistinguishesFromSequence(t *testing.T) {
	svc := buildSequenceGraph(t)

	res, err := svc.Search(`\story step 1`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\story step 1: expected chains; got none`)
	}
	for i, chain := range res.Structural {
		if chain.Kind != "sequence" {
			t.Errorf("chain %d: expected Kind=sequence; got %q", i, chain.Kind)
		}
	}
}

// ── full notes via PageMap (plan feature) ─────────────────────────────────────

// buildNotesGraph builds a graph from notes.n4l plus optionally notes_two.n4l,
// returning a SearchService backed by the BadgerDB store at dir/sst_data.  The
// dir is returned too so tests can close/reopen the service against the same
// store (persistence tests).
func buildNotesGraph(t *testing.T, includeTwo bool) (*search.SearchService, string) {
	t.Helper()
	dir := t.TempDir()

	copyFixture := func(name string) {
		data, err := os.ReadFile(filepath.Join(testFixturesDir(t), name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	copyFixture("notes.n4l")
	if includeTwo {
		copyFixture("notes_two.n4l")
	}

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)

	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)
	return svc, dir
}

func TestNotes_DittoFlatten(t *testing.T) {
	svc, _ := buildNotesGraph(t, false)

	res, err := svc.Search(`\notes brain`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	alpha := findNoteBlock(res.Notes, "alpha waves")
	if alpha == nil {
		t.Fatalf("alpha waves block missing; got %v", noteTexts(res.Notes))
	}
	if len(alpha.Arrows) != 2 {
		t.Fatalf("alpha waves: expected 2 arrows (root freq + ditto state); got %d: %+v", len(alpha.Arrows), alpha.Arrows)
	}
	// Arrows[0] should be (freq) 5-15 Hz from the root line.
	if alpha.Arrows[0].ArrowCode != "freq" || alpha.Arrows[0].Target != "5-15 Hz" {
		t.Errorf("alpha Arrows[0]: want freq → 5-15 Hz; got %q → %q", alpha.Arrows[0].ArrowCode, alpha.Arrows[0].Target)
	}
	// Arrows[1] is the ditto (state) very relaxed — same block, next line.
	if alpha.Arrows[1].ArrowCode != "state" || alpha.Arrows[1].Target != "very relaxed" {
		t.Errorf("alpha Arrows[1] (ditto): want state → very relaxed; got %q → %q", alpha.Arrows[1].ArrowCode, alpha.Arrows[1].Target)
	}
	if alpha.Arrows[1].ArrowLong != "has state" {
		t.Errorf("alpha ditto ArrowLong: want %q; got %q", "has state", alpha.Arrows[1].ArrowLong)
	}
}

func TestNotes_ContextFilter_UnionSubtract(t *testing.T) {
	svc, _ := buildNotesGraph(t, false)

	res, err := svc.Search(`\notes brain \context high-freq`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !hasNoteBlock(res.Notes, "gamma waves") {
		t.Errorf(`\notes brain \context high-freq: expected "gamma waves"; got %v`, noteTexts(res.Notes))
	}
	for _, unwanted := range []string{"alpha waves", "beta waves", "neocortex"} {
		if hasNoteBlock(res.Notes, unwanted) {
			t.Errorf(`\notes brain \context high-freq: unexpected %q (union/subtract should have cleared it); got %v`, unwanted, noteTexts(res.Notes))
		}
	}
}

func TestNotes_ContextFilter_Replace(t *testing.T) {
	svc, _ := buildNotesGraph(t, false)

	res, err := svc.Search(`\notes brain \context "brain waves"`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// :: brain waves, oscillations :: applies to alpha/beta; +:: high-freq :: is
	// a union so gamma keeps "brain waves" too. :: structure :: replaces the
	// set, so neocortex must NOT appear.
	for _, want := range []string{"alpha waves", "beta waves", "gamma waves"} {
		if !hasNoteBlock(res.Notes, want) {
			t.Errorf(`\notes brain \context "brain waves": expected %q; got %v`, want, noteTexts(res.Notes))
		}
	}
	if hasNoteBlock(res.Notes, "neocortex") {
		t.Errorf(`\notes brain \context "brain waves": unexpected "neocortex" (:: structure :: should have cleared the tag); got %v`, noteTexts(res.Notes))
	}
}

func TestNotes_MultiChapter(t *testing.T) {
	svc, _ := buildNotesGraph(t, true)

	res, err := svc.Search(`\notes any`)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Notes) < 2 {
		t.Fatalf(`\notes any: expected ≥2 FullNotes (one per chapter); got %d`, len(res.Notes))
	}
	chapters := make(map[string]bool, len(res.Notes))
	for _, n := range res.Notes {
		chapters[n.Chapter] = true
	}
	for _, want := range []string{"brain notes", "body notes"} {
		if !chapters[want] {
			t.Errorf(`\notes any: missing chapter %q; got %v`, want, chapters)
		}
	}
	// Blocks must come from the right chapter.
	if blk := findNoteBlock(res.Notes, "heart"); blk == nil {
		t.Errorf(`\notes any: "heart" block missing`)
	}
	if blk := findNoteBlock(res.Notes, "alpha waves"); blk == nil {
		t.Errorf(`\notes any: "alpha waves" block missing`)
	}
}

func TestNotes_PersistsAcrossRestart(t *testing.T) {
	svc, dir := buildNotesGraph(t, false)

	first, err := svc.Search(`\notes brain`)
	if err != nil {
		t.Fatalf("first Search: %v", err)
	}
	firstTexts := noteTexts(first.Notes)
	if len(firstTexts) == 0 {
		t.Fatalf("first \\notes brain returned nothing")
	}

	// Close the service (releases Badger lock) and reopen a fresh one against
	// the same dir — no rebuild.  If PageMap is persisted, results must match.
	svc.Close()

	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	svc2 := search.NewSearchService(dir, reg)
	t.Cleanup(svc2.Close)

	second, err := svc2.Search(`\notes brain`)
	if err != nil {
		t.Fatalf("second Search: %v", err)
	}
	secondTexts := noteTexts(second.Notes)

	sort.Strings(firstTexts)
	sort.Strings(secondTexts)
	if strings.Join(firstTexts, "|") != strings.Join(secondTexts, "|") {
		t.Errorf("notes mismatch across restart:\n  before: %v\n   after: %v", firstTexts, secondTexts)
	}
}

func TestNotes_ChapterReingest(t *testing.T) {
	svc, dir := buildNotesGraph(t, false)

	first, err := svc.Search(`\notes brain`)
	if err != nil {
		t.Fatalf("first Search: %v", err)
	}
	firstCount := 0
	for _, n := range first.Notes {
		firstCount += len(n.Blocks)
	}
	if firstCount == 0 {
		t.Fatalf("first \\notes brain returned no blocks")
	}

	// Rebuild — BuildFromCollection wipes <dir>/sst_data entirely, so stale
	// pm: keys from the previous pass must not survive to leak duplicate
	// PageMap rows.
	svc.Close()
	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("re-ingest BuildFromCollection: %v", err)
	}
	svc2 := search.NewSearchService(dir, reg)
	t.Cleanup(svc2.Close)

	second, err := svc2.Search(`\notes brain`)
	if err != nil {
		t.Fatalf("second Search: %v", err)
	}
	secondCount := 0
	for _, n := range second.Notes {
		secondCount += len(n.Blocks)
	}
	if secondCount != firstCount {
		t.Errorf("block count changed after re-ingest: before=%d after=%d", firstCount, secondCount)
	}
}

// buildStemFixtureGraph writes a tiny English fixture with a single
// "running" node, builds the graph (BadgerDB + Bleve), and returns a ready
// SearchService. Used by TestSearchService_SearchNodes_StemMatch to verify
// that the Bleve overlay path returns morphological variants for the
// English Snowball stemmer (FR-013, US1).
func buildStemFixtureGraph(t *testing.T) *search.SearchService {
	t.Helper()
	dir := t.TempDir()
	src := "- English\n\nrunning\n\"  (means) jogging\n"
	if err := os.WriteFile(filepath.Join(dir, "english.n4l"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}
	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)
	return svc
}

// TestSearchService_SearchNodes_StemMatch verifies the Bleve overlay returns
// `running` when the user queries `runs`. Will fail until builder.go indexes
// nodes into Bleve and SearchService.searchNodes routes through SearchByQuery.
func TestSearchService_SearchNodes_StemMatch(t *testing.T) {
	svc := buildStemFixtureGraph(t)
	res, err := svc.Search("runs")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.GraphUnavailable {
		t.Fatal("graph unavailable")
	}
	found := false
	for _, n := range res.Nodes {
		if strings.EqualFold(n.NodeText, "running") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("stem-form 'runs' did not return 'running'; got %v", nodeTexts(res.Nodes))
	}
}

// buildPhraseBoolGraph writes a fixture covering both phrase- and
// boolean-operator scenarios so a single graph can drive several US2
// assertions through the Texere overlay.
func buildPhraseBoolGraph(t *testing.T) *search.SearchService {
	t.Helper()
	dir := t.TempDir()
	src := "- Recipes\n\nfish soup recipe\n\nfish curry and soup\n\nbrain research\n\nbrain notes\n\nnotes about music\n"
	if err := os.WriteFile(filepath.Join(dir, "recipes.n4l"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}
	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)
	return svc
}

// TestSearchService_SearchNodes_PhraseAndBoolean drives the full overlay
// path for two US2 acceptance scenarios — phrase match and AND-NOT — to
// catch any wiring regression between SearchByQuery's parse error / result
// surfaces and SearchService.searchNodes.
func TestSearchService_SearchNodes_PhraseAndBoolean(t *testing.T) {
	svc := buildPhraseBoolGraph(t)

	t.Run("phrase", func(t *testing.T) {
		res, err := svc.Search(`"fish soup"`)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.GraphUnavailable {
			t.Fatal("graph unavailable")
		}
		texts := nodeTexts(res.Nodes)
		hasRecipe := false
		hasCurry := false
		for _, txt := range texts {
			if strings.EqualFold(txt, "fish soup recipe") {
				hasRecipe = true
			}
			if strings.EqualFold(txt, "fish curry and soup") {
				hasCurry = true
			}
		}
		if !hasRecipe {
			t.Errorf(`phrase "fish soup" did not return "fish soup recipe"; got %v`, texts)
		}
		if hasCurry {
			t.Errorf(`phrase "fish soup" must not match disjoint "fish curry and soup"; got %v`, texts)
		}
	})

	t.Run("and-not", func(t *testing.T) {
		res, err := svc.Search("brain&!notes")
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if res.GraphUnavailable {
			t.Fatal("graph unavailable")
		}
		texts := nodeTexts(res.Nodes)
		hasResearch := false
		hasBrainNotes := false
		hasNotesOnly := false
		for _, txt := range texts {
			switch strings.ToLower(txt) {
			case "brain research":
				hasResearch = true
			case "brain notes":
				hasBrainNotes = true
			case "notes about music":
				hasNotesOnly = true
			}
		}
		if !hasResearch {
			t.Errorf(`brain&!notes should include "brain research"; got %v`, texts)
		}
		if hasBrainNotes {
			t.Errorf(`brain&!notes must exclude "brain notes"; got %v`, texts)
		}
		if hasNotesOnly {
			t.Errorf(`brain&!notes must exclude "notes about music"; got %v`, texts)
		}
	})
}

// ── US3 regression: zero-regression on `\` commands ──────────────────────────

// panicBleveStub satisfies search.BleveSearcher and panics on SearchByQuery.
// Used by US3 tests to assert that listing/notes/wildcard routes never reach
// the index (FR-004).
type panicBleveStub struct{}

func (panicBleveStub) SearchByQuery(string, int) ([]SST.NodePtr, error) {
	panic("BleveSearcher.SearchByQuery should not be called for routes that bypass Bleve")
}

func (panicBleveStub) Close() error { return nil }

// TestSearchService_StructuralSequence_Bleve guards FR-023a chain ordering on
// a `\sequence` query: replacing the sequence-branch seed lookup with Bleve
// must not reorder chains (chain order is governed by GetSequenceContainers /
// the score-stable resort, not Bleve). We assert the sequence head node and
// step set survive end-to-end.
func TestSearchService_StructuralSequence_Bleve(t *testing.T) {
	svc := buildSequenceGraph(t)

	res, err := svc.Search(`\story step 1`)
	if err != nil {
		t.Fatalf(`\story step 1: %v`, err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\story step 1: expected at least one chain; got none`)
	}
	chain := res.Structural[0]
	texts := chainTexts(chain)
	for _, want := range []string{"step 1", "step 2", "step 3"} {
		found := false
		for _, txt := range texts {
			if strings.Contains(strings.ToLower(txt), want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("\\story step 1 (Bleve): expected %q in chain; got %v", want, texts)
		}
	}
}

// TestSearchService_ResolvePathEndpoints_Bleve guards \paths endpoint
// resolution under the Bleve fallback: textToDBPtr exact-match still wins
// over Bleve seeds, and the path enumeration ordering is preserved.
func TestSearchService_ResolvePathEndpoints_Bleve(t *testing.T) {
	svc := buildPathsGraph(t)

	res, err := svc.Search(`\from start \to goal`)
	if err != nil {
		t.Fatalf(`\from start \to goal: %v`, err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\from start \to goal: expected at least one path; got none`)
	}
	for i, chain := range res.Structural {
		if chain.Kind != "path" {
			t.Errorf("chain %d: Kind=%q want path", i, chain.Kind)
		}
		texts := chainTexts(chain)
		if len(texts) < 2 {
			t.Errorf("chain %d: %d nodes < 2", i, len(texts))
			continue
		}
		if !strings.EqualFold(texts[0], "start") {
			t.Errorf("chain %d: head=%q want exact 'start'; chain=%v", i, texts[0], texts)
		}
		if !strings.EqualFold(texts[len(texts)-1], "goal") {
			t.Errorf("chain %d: tail=%q want exact 'goal'; chain=%v", i, texts[len(texts)-1], texts)
		}
	}
}

// TestSearchService_ForwardCone covers a lone `\from` (no `\to`): upstream
// SSTorytime treats this as a FORWARD causal cone, not a node-text search.
// Before the fix it fell through to the node-search wildcard and dumped the
// whole graph; now it must return focused Structural chains rooted at the
// endpoint and reaching the cone's leaves.
func TestSearchService_ForwardCone(t *testing.T) {
	svc := buildPathsGraph(t)

	res, err := svc.Search(`\from start`)
	if err != nil {
		t.Fatalf(`\from start: %v`, err)
	}
	// A lone \from is a cone, not a node search — results land in Structural.
	if len(res.Nodes) != 0 {
		t.Errorf(`\from start: expected no node-search results (cone routes to Structural); got %v`, nodeTexts(res.Nodes))
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\from start: expected at least one cone chain; got none`)
	}

	// The cone is emitted as a layered DAG of one edge per chain. "start" must be
	// the unique source (it appears as an edge head but NEVER as a destination —
	// the layout relies on its in-degree being 0 to root the flow). The cone must
	// reach "goal" and must not leak nodes outside the four-node Maze.
	mazeNodes := map[string]bool{"start": true, "door": true, "passage": true, "goal": true}
	reached := map[string]bool{}
	startIsDest := false
	for i, chain := range res.Structural {
		if chain.Kind != "path" {
			t.Errorf("chain %d: Kind=%q want path", i, chain.Kind)
		}
		texts := chainTexts(chain)
		if len(texts) < 2 {
			t.Errorf("chain %d: %d nodes < 2", i, len(texts))
			continue
		}
		for j, txt := range texts {
			reached[strings.ToLower(txt)] = true
			if !mazeNodes[strings.ToLower(txt)] {
				t.Errorf("chain %d: cone leaked unrelated node %q; chain=%v", i, txt, texts)
			}
			if j > 0 && strings.EqualFold(txt, "start") {
				startIsDest = true // an edge points back into the root
			}
		}
	}
	if !reached["start"] {
		t.Errorf(`\from start: "start" missing from cone`)
	}
	if startIsDest {
		t.Errorf(`\from start: "start" must be the root (in-degree 0), but an edge points back to it`)
	}
	if !reached["goal"] {
		t.Errorf(`\from start: cone never reached "goal"; reached=%v`, reached)
	}
}

// TestSearchService_ExactConeEndpoint guards the exact-match contract on cone
// endpoints: `\from !start!` resolves ONLY to a node literally named "start",
// and `\from !nope!` (no such node) returns nothing rather than fuzzy-falling
// back to unrelated Bleve hits that would seed cones at the wrong roots.
func TestSearchService_ExactConeEndpoint(t *testing.T) {
	svc := buildPathsGraph(t)

	// Exact match that exists → a cone rooted at "start".
	res, err := svc.Search(`\from !start!`)
	if err != nil {
		t.Fatalf(`\from !start!: %v`, err)
	}
	if len(res.Structural) == 0 {
		t.Fatal(`\from !start!: expected a cone; got none`)
	}
	// "start" must anchor the cone: present as a source, never as a destination.
	startSeen, startIsDest := false, false
	for _, chain := range res.Structural {
		texts := chainTexts(chain)
		for j, txt := range texts {
			if strings.EqualFold(txt, "start") {
				startSeen = true
				if j > 0 {
					startIsDest = true
				}
			}
		}
	}
	if !startSeen {
		t.Errorf(`\from !start!: cone does not include "start"`)
	}
	if startIsDest {
		t.Errorf(`\from !start!: "start" must be the cone root, but an edge points back to it`)
	}

	// Exact match that does NOT exist → empty, no fuzzy substitutes.
	res, err = svc.Search(`\from !definitelynotanode!`)
	if err != nil {
		t.Fatalf(`\from !definitelynotanode!: %v`, err)
	}
	if len(res.Structural) != 0 {
		t.Errorf(`\from !definitelynotanode!: exact miss must yield no chains; got %d`, len(res.Structural))
	}
}

// TestSearchService_RouteListing_NoBleveCall asserts that `\chapter X`
// completes without invoking the Bleve index (FR-004). After lazy-open we
// swap in a stub that panics on SearchByQuery; the listing route must not
// trigger it.
func TestSearchService_RouteListing_NoBleveCall(t *testing.T) {
	svc := buildTestGraph(t)
	if _, err := svc.Search(`primer`); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	svc.SetBleveSearcherForTest(panicBleveStub{})

	res, err := svc.Search(`\chapter Chinese`)
	if err != nil {
		t.Fatalf(`\chapter Chinese: %v`, err)
	}
	if len(res.Listings) == 0 {
		t.Errorf(`\chapter Chinese: expected at least one listing; got none`)
	}
}

// TestSearchService_RouteNotes_NoBleveCall asserts that `\notes` queries do
// not reach the Bleve index.
func TestSearchService_RouteNotes_NoBleveCall(t *testing.T) {
	svc := buildCookingGraph(t)
	if _, err := svc.Search(`primer`); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	svc.SetBleveSearcherForTest(panicBleveStub{})

	res, err := svc.Search(`\notes Cooking`)
	if err != nil {
		t.Fatalf(`\notes Cooking: %v`, err)
	}
	if len(res.Notes) == 0 {
		t.Errorf(`\notes Cooking: expected at least one FullNote; got none`)
	}
}

// TestSearchService_EmptyName_BypassesBleve covers FR-021: a query with no
// text term but a `\chapter` filter must use the IterateNodes path, not Bleve.
// The classifier routes such input to routeListing; this test locks in that
// behavior with the panic stub.
func TestSearchService_EmptyName_BypassesBleve(t *testing.T) {
	svc := buildTestGraph(t)
	if _, err := svc.Search(`primer`); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	svc.SetBleveSearcherForTest(panicBleveStub{})

	if _, err := svc.Search(`\chapter Chinese`); err != nil {
		t.Errorf(`\chapter Chinese (no name): %v`, err)
	}
}

// TestSearchService_LegacyQueryRegression covers T058 / SC-001 / FR-014:
// the Bleve-backed Search must return at least every (NodeText, Chapter) pair
// that the pre-Bleve implementation returned for representative queries.
// Extra hits are allowed (e.g. stemming finds "soups" for "soup"); missing
// hits are a regression. Baseline captured from commit 173f159 (the branch-cut
// for feature 005) via cmd/baseline_capture (now removed); JSON committed at
// docs/specs/005-bleve-fulltext-search/baseline/legacy_queries.json.
func TestSearchService_LegacyQueryRegression(t *testing.T) {
	type baselineHit struct {
		NodeText string `json:"NodeText"`
		Chapter  string `json:"Chapter"`
	}
	type baselineEntry struct {
		Query string        `json:"Query"`
		Hits  []baselineHit `json:"Hits"`
	}

	// Resolve baseline JSON relative to the repo root (this test file's
	// directory is internal/search/, so go up two).
	_, file, _, _ := runtime.Caller(0)
	baselinePath := filepath.Join(filepath.Dir(file), "..", "..",
		"docs", "specs", "005-bleve-fulltext-search", "baseline", "legacy_queries.json")

	data, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline %s: %v", baselinePath, err)
	}
	var baseline []baselineEntry
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatalf("unmarshal baseline: %v", err)
	}

	// Build the same fixtures used to capture the baseline.
	dir := t.TempDir()
	fix := testFixturesDir(t)
	for _, name := range []string{"chinese.n4l", "cooking.n4l", "paths.n4l", "sequence.n4l", "implicit_sequence.n4l", "notes.n4l", "notes_two.n4l"} {
		body, err := os.ReadFile(filepath.Join(fix, name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	reg := testRegistry()
	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatalf("BuildFromCollection: %v", err)
	}
	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)

	for _, entry := range baseline {
		entry := entry // capture
		t.Run(entry.Query, func(t *testing.T) {
			res, err := svc.Search(entry.Query)
			if err != nil {
				t.Fatalf("Search %q: %v", entry.Query, err)
			}
			got := make(map[string]struct{}, len(res.Nodes))
			for _, n := range res.Nodes {
				got[n.NodeText+"|"+n.Chapter] = struct{}{}
			}
			for _, want := range entry.Hits {
				key := want.NodeText + "|" + want.Chapter
				if _, ok := got[key]; !ok {
					t.Errorf("regression: query %q missing baseline hit (NodeText=%q Chapter=%q)", entry.Query, want.NodeText, want.Chapter)
				}
			}
		})
	}
}

// TestSearchService_OpenCorruptIndex_SurfacesError covers T059: when the
// bleve/ directory exists but its contents are unreadable, Search must surface
// `result.GraphError` containing the corrupt-index sentinel so the OmniBar
// can render the "Press Ctrl+G to rebuild" toast.
func TestSearchService_OpenCorruptIndex_SurfacesError(t *testing.T) {
	// First build a valid graph so sst_data/ exists, then nuke bleve/ and
	// replace it with garbage bytes that Bleve cannot decode.
	svc, dir := buildTestGraphInDir(t)
	if _, err := svc.Search(`primer`); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	svc.Close() // release Bleve so we can overwrite the dir

	blevePath := filepath.Join(dir, "bleve")
	if err := os.RemoveAll(blevePath); err != nil {
		t.Fatalf("remove bleve: %v", err)
	}
	if err := os.MkdirAll(blevePath, 0755); err != nil {
		t.Fatalf("mkdir bleve: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blevePath, "garbage.bolt"), []byte("not a real bleve store"), 0644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	// Re-open the service against the corrupted overlay.
	svc2 := search.NewSearchService(dir, testRegistry())
	t.Cleanup(svc2.Close)

	if !svc2.IsReady() {
		t.Fatal("IsReady() = false; corrupt-bleve dir still exists, want true")
	}

	res, err := svc2.Search("anything")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !res.GraphUnavailable {
		t.Errorf("GraphUnavailable = false; want true on corrupt index")
	}
	if !strings.Contains(res.GraphError, "corrupted") {
		t.Errorf("GraphError = %q; want substring 'corrupted'", res.GraphError)
	}
}

// TestSearchService_IsReady_RequiresBothDirs locks in FR-019b: a SearchService
// is ready only when BOTH the BadgerDB (sst_data/) and the Bleve index (bleve/)
// directories exist on disk. A half-built collection (one missing) must report
// not-ready so the caller surfaces the same "press Ctrl+G" toast.
func TestSearchService_IsReady_RequiresBothDirs(t *testing.T) {
	dir := t.TempDir()
	reg := testRegistry()

	// State 1: neither directory present.
	svc := search.NewSearchService(dir, reg)
	if svc.IsReady() {
		t.Errorf("IsReady() with no dirs = true; want false")
	}

	// State 2: only sst_data/ present (simulates aborted build before Bleve flush).
	if err := os.MkdirAll(filepath.Join(dir, "sst_data"), 0755); err != nil {
		t.Fatalf("mkdir sst_data: %v", err)
	}
	if svc.IsReady() {
		t.Errorf("IsReady() with only sst_data/ = true; want false")
	}

	// State 3: both dirs present.
	if err := os.MkdirAll(filepath.Join(dir, "bleve"), 0755); err != nil {
		t.Fatalf("mkdir bleve: %v", err)
	}
	if !svc.IsReady() {
		t.Errorf("IsReady() with both dirs = false; want true")
	}
}
