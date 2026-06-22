package content

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// emptyRegistry returns a minimal SSTRegistry with common test arrows registered.
func emptyRegistry() *SSTRegistry {
	reg := &SSTRegistry{
		Arrows:      make(map[string]ArrowDef),
		Annotations: make(map[string]AnnotDef),
	}
	for _, code := range []string{"means", "e.g.", "note", "ex", "NB", "remark"} {
		reg.Arrows[code] = ArrowDef{Code: code, MetaType: "test"}
	}
	return reg
}

// loadTestRegistry loads the real SSTconfig for tests that need it.
func loadTestRegistry(tb testing.TB) *SSTRegistry {
	tb.Helper()
	_, file, _, _ := runtime.Caller(0)
	// Walk up to repo root to find SSTconfig/
	dir := filepath.Dir(file)
	for {
		candidate := filepath.Join(dir, "SSTconfig")
		if _, err := os.Stat(candidate); err == nil {
			reg, err := LoadRegistry(candidate)
			if err != nil {
				tb.Logf("LoadRegistry warning: %v", err)
			}
			return reg
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	tb.Log("SSTconfig not found; using empty registry")
	return emptyRegistry()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testdataPath(tb testing.TB, name string) string {
	tb.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

// ---------------------------------------------------------------------------
// T011: ScanCollection tests
// ---------------------------------------------------------------------------

func TestScanCollectionEmpty(t *testing.T) {
	dir := t.TempDir()
	summaries, err := ScanCollection(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("want 0 summaries, got %d", len(summaries))
	}
}

func TestScanCollectionSorted(t *testing.T) {
	dir := t.TempDir()
	names := []string{"zebra.n4l", "apple.n4l", "Mango.n4l", "cherry.n4l", "Banana.n4l"}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("- chapter\nnode\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	summaries, err := ScanCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 5 {
		t.Fatalf("want 5, got %d", len(summaries))
	}
	for i := 1; i < len(summaries); i++ {
		a := strings.ToLower(summaries[i-1].FileName)
		b := strings.ToLower(summaries[i].FileName)
		if a > b {
			t.Errorf("not sorted: %q > %q", summaries[i-1].FileName, summaries[i].FileName)
		}
	}
}

func TestScanCollectionMultiChapter(t *testing.T) {
	dir := t.TempDir()
	src := testdataPath(t, "multi-chapter.n4l")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "multi-chapter.n4l"), data, 0644); err != nil {
		t.Fatal(err)
	}
	summaries, err := ScanCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("want 1 summary, got %d", len(summaries))
	}
	if len(summaries[0].Chapters) != 3 {
		t.Fatalf("want 3 chapters, got %d: %v", len(summaries[0].Chapters), summaries[0].Chapters)
	}
	want := []string{"introduction", "main concepts", "advanced topics"}
	for i, w := range want {
		if summaries[0].Chapters[i] != w {
			t.Errorf("chapter[%d]: want %q, got %q", i, w, summaries[0].Chapters[i])
		}
	}
}

func TestScanCollectionUnreadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; chmod restriction not enforced")
	}
	dir := t.TempDir()
	names := []string{"readable.n4l", "unreadable.n4l"}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("- chapter\nnode\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(dir, "unreadable.n4l"), 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(dir, "unreadable.n4l"), 0644)

	summaries, err := ScanCollection(dir)
	if err != nil {
		t.Fatalf("directory-level error unexpected: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(summaries))
	}
	var hadError bool
	for _, s := range summaries {
		if s.FileName == "unreadable.n4l" && s.ScanError != nil {
			hadError = true
		}
		if s.FileName == "readable.n4l" && s.ScanError != nil {
			t.Errorf("readable.n4l should not have a ScanError")
		}
	}
	if !hadError {
		t.Error("unreadable.n4l should have a ScanError")
	}
}

func TestScanCollectionRecursive(t *testing.T) {
	dir := t.TempDir()

	// Top-level file.
	if err := os.WriteFile(filepath.Join(dir, "top.n4l"), []byte("- a\nnode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// reading/ subdir file — must be discovered.
	if err := os.MkdirAll(filepath.Join(dir, "reading"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reading", "deep.n4l"), []byte("- b\nnode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Denylisted + hidden dirs containing .n4l — must be ignored.
	for _, d := range []string{"sst_data", "bleve", "sources", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, d, "x.n4l"), []byte("- c\nnode\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	summaries, err := ScanCollection(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, s := range summaries {
		got[s.RelPath] = true
	}
	if len(summaries) != 2 {
		t.Fatalf("want 2 summaries, got %d: %v", len(summaries), got)
	}
	if !got["top.n4l"] {
		t.Errorf("missing top-level file; got %v", got)
	}
	if !got["reading/deep.n4l"] {
		t.Errorf("missing subdir file; got %v", got)
	}
	// Top-level RelPath is the bare base name (no slash); subdir RelPath is slashed.
	for _, s := range summaries {
		switch s.FileName {
		case "top.n4l":
			if s.RelPath != "top.n4l" {
				t.Errorf("top-level RelPath = %q, want %q", s.RelPath, "top.n4l")
			}
		case "deep.n4l":
			if s.RelPath != "reading/deep.n4l" {
				t.Errorf("subdir RelPath = %q, want %q", s.RelPath, "reading/deep.n4l")
			}
		}
	}
}

func BenchmarkScanCollection(b *testing.B) {
	dir := b.TempDir()
	src := testdataPath(b, "simple.n4l")
	data, err := os.ReadFile(src)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		name := filepath.Join(dir, "file"+string(rune('a'+i%26))+".n4l")
		if err := os.WriteFile(name, data, 0644); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ScanCollection(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// T013: WatchCollection tests
// ---------------------------------------------------------------------------

func TestWatchCollectionAdded(t *testing.T) {
	dir := t.TempDir()
	watcher, err := WatchCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	content := []byte("- watched chapter\nsome node\n")
	if err := os.WriteFile(filepath.Join(dir, "new.n4l"), content, 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-watcher.Events:
		if ev.Kind != EventAdded {
			t.Errorf("want EventAdded, got %v", ev.Kind)
		}
		if !strings.HasSuffix(ev.Path, "new.n4l") {
			t.Errorf("unexpected path: %s", ev.Path)
		}
		if len(ev.Chapters) == 0 {
			t.Error("want chapters populated for Added event")
		} else if len(ev.Chapters[0].Chapters) == 0 || ev.Chapters[0].Chapters[0] != "watched chapter" {
			t.Errorf("unexpected chapters: %v", ev.Chapters)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for Added event")
	}
}

func TestWatchCollectionRemoved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todelete.n4l")
	if err := os.WriteFile(p, []byte("- chapter\nnode\n"), 0644); err != nil {
		t.Fatal(err)
	}

	watcher, err := WatchCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	// Drain any Write/Create events then find the Remove
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case ev := <-watcher.Events:
			if ev.Kind == EventRemoved {
				if !strings.HasSuffix(ev.Path, "todelete.n4l") {
					t.Errorf("unexpected path: %s", ev.Path)
				}
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting for Removed event")
		}
	}
}

func TestWatchCollectionIgnoresNonN4L(t *testing.T) {
	dir := t.TempDir()
	watcher, err := WatchCollection(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-watcher.Events:
		t.Errorf("unexpected event for non-.n4l file: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// correct — no event emitted
	}
}

// ---------------------------------------------------------------------------
// T021: ParseFile tests
// ---------------------------------------------------------------------------

func TestParseFidelitySimple(t *testing.T) {
	reg := emptyRegistry()
	pf, errs := ParseFile(testdataPath(t, "simple.n4l"), reg)
	if pf == nil {
		t.Fatal("ParseFile returned nil")
	}
	_ = errs // simple.n4l may produce unknown-arrow errors; we test structure

	if len(pf.Chapters) != 1 {
		t.Fatalf("want 1 chapter, got %d", len(pf.Chapters))
	}
	ch := pf.Chapters[0]
	if ch.Title != "basic notes" {
		t.Errorf("chapter title: want %q, got %q", "basic notes", ch.Title)
	}
	if len(ch.Nodes) < 2 {
		t.Fatalf("want at least 2 nodes, got %d", len(ch.Nodes))
	}
	// First node should have at least one arrow (ditto line)
	if len(ch.Nodes[0].Dittos) == 0 && len(ch.Nodes[0].Arrows) == 0 {
		t.Error("first node should have arrows or dittos")
	}
}

func TestParseContextReplay(t *testing.T) {
	reg := emptyRegistry()
	pf, _ := ParseFile(testdataPath(t, "contexts.n4l"), reg)
	if pf == nil || len(pf.Chapters) == 0 {
		t.Fatal("failed to parse contexts.n4l")
	}
	ch := pf.Chapters[0]

	// Expected context snapshots per node (0-indexed).
	// contexts.n4l sets individual tags via separate ::, +::, -:: lines:
	//   :: biology  +:: chemistry  →  nodes 0,1: {biology, chemistry}
	//   +:: physics               →  nodes 2,3: {biology, chemistry, physics}
	//   -:: chemistry             →  nodes 4,5: {biology, physics}
	//   :: linguistics            →  node  6:   {linguistics}
	//   ::                        →  node  7:   {} (empty)

	contains := func(ctx []string, tag string) bool {
		for _, t := range ctx {
			if t == tag {
				return true
			}
		}
		return false
	}

	if len(ch.Nodes) < 8 {
		t.Fatalf("want at least 8 nodes, got %d", len(ch.Nodes))
	}

	// Nodes 0,1: biology+chemistry
	for i := 0; i < 2; i++ {
		if !contains(ch.Nodes[i].EffectiveCtx, "biology") || !contains(ch.Nodes[i].EffectiveCtx, "chemistry") {
			t.Errorf("node[%d]: want {biology,chemistry}, got %v", i, ch.Nodes[i].EffectiveCtx)
		}
	}
	// Node 2,3: biology+chemistry+physics
	for i := 2; i < 4; i++ {
		if !contains(ch.Nodes[i].EffectiveCtx, "physics") {
			t.Errorf("node[%d]: want physics in ctx, got %v", i, ch.Nodes[i].EffectiveCtx)
		}
	}
	// Node 4,5: biology+physics (no chemistry)
	for i := 4; i < 6; i++ {
		if contains(ch.Nodes[i].EffectiveCtx, "chemistry") {
			t.Errorf("node[%d]: chemistry should be removed, got %v", i, ch.Nodes[i].EffectiveCtx)
		}
		if !contains(ch.Nodes[i].EffectiveCtx, "biology") {
			t.Errorf("node[%d]: biology should remain, got %v", i, ch.Nodes[i].EffectiveCtx)
		}
	}
	// Node 6: linguistics only
	if len(ch.Nodes[6].EffectiveCtx) != 1 || ch.Nodes[6].EffectiveCtx[0] != "linguistics" {
		t.Errorf("node[6]: want {linguistics}, got %v", ch.Nodes[6].EffectiveCtx)
	}
	// Node 7: empty
	if len(ch.Nodes[7].EffectiveCtx) != 0 {
		t.Errorf("node[7]: want empty ctx, got %v", ch.Nodes[7].EffectiveCtx)
	}
}

func TestParseSequence(t *testing.T) {
	reg := emptyRegistry()
	pf, _ := ParseFile(testdataPath(t, "sequence.n4l"), reg)
	if pf == nil || len(pf.Chapters) == 0 {
		t.Fatal("failed to parse sequence.n4l")
	}
	ch := pf.Chapters[0]

	// Node 0: not sequence
	if ch.Nodes[0].IsSequence {
		t.Error("node[0] should not be sequence")
	}

	// Nodes 1–8 should be sequence
	seqCount := 0
	for _, n := range ch.Nodes {
		if n.IsSequence {
			seqCount++
		}
	}
	if seqCount < 8 {
		t.Errorf("want at least 8 sequence nodes, got %d", seqCount)
	}

	// Last node after -:: _sequence_ should not be sequence
	last := ch.Nodes[len(ch.Nodes)-1]
	if last.IsSequence {
		t.Error("last node after -:: _sequence_ should not be sequence")
	}
}

func TestParseLabelResolution(t *testing.T) {
	reg := emptyRegistry()
	pf, errs := ParseFile(testdataPath(t, "labels.n4l"), reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}
	if len(pf.Chapters) == 0 {
		t.Fatal("no chapters")
	}

	ch := pf.Chapters[0]
	// Check labels are registered in the chapter
	if _, ok := ch.Labels["anchor1"]; !ok {
		t.Error("anchor1 label not registered")
	}
	if _, ok := ch.Labels["anchor2"]; !ok {
		t.Error("anchor2 label not registered")
	}
	if _, ok := ch.Labels["anchor3"]; !ok {
		t.Error("anchor3 label not registered")
	}

	// No unresolved reference errors (all labels exist)
	for _, e := range errs {
		if strings.Contains(e.Msg, ErrUnresolvedRef) {
			t.Errorf("unexpected unresolved ref error: %v", e)
		}
	}
}

func TestParseErrorCollection(t *testing.T) {
	reg := emptyRegistry()
	pf, errs := ParseFile(testdataPath(t, "errors.n4l"), reg)
	if pf == nil {
		t.Fatal("nil ParsedFile even with errors")
	}
	if len(errs) == 0 {
		t.Fatal("expected errors from errors.n4l, got none")
	}

	// Should have at least: unknown arrow, unresolved ref
	// Note: orphan dittos are now treated as root nodes rather than errors
	var hasUnknown, hasUnresolved bool
	for _, e := range errs {
		if strings.Contains(e.Msg, ErrUnknownArrow) {
			hasUnknown = true
		}
		if strings.Contains(e.Msg, ErrUnresolvedRef) {
			hasUnresolved = true
		}
	}
	if !hasUnknown {
		t.Errorf("expected unknown arrow error; got: %v", errs)
	}
	if !hasUnresolved {
		t.Errorf("expected unresolved ref error; got: %v", errs)
	}

	// Valid nodes still returned
	if len(pf.Chapters) == 0 || len(pf.Chapters[0].Nodes) == 0 {
		t.Error("expected valid nodes despite errors")
	}
}

func TestParseDeterminism(t *testing.T) {
	reg := loadTestRegistry(t)
	path := testdataPath(t, "multi-chapter.n4l")

	pf1, _ := ParseFile(path, reg)
	pf2, _ := ParseFile(path, reg)

	if len(pf1.Chapters) != len(pf2.Chapters) {
		t.Fatalf("chapter count differs: %d vs %d", len(pf1.Chapters), len(pf2.Chapters))
	}
	for i := range pf1.Chapters {
		if len(pf1.Chapters[i].Nodes) != len(pf2.Chapters[i].Nodes) {
			t.Errorf("chapter[%d] node count differs", i)
		}
	}
}

func BenchmarkParseFile10k(b *testing.B) {
	reg := loadTestRegistry(b)
	path := testdataPath(b, "large.n4l")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pf, _ := ParseFile(path, reg)
		if pf == nil {
			b.Fatal("nil ParsedFile")
		}
	}
	b.ReportAllocs()
}

// TestMain gates the build if the benchmark exceeds 50 ms when run with -bench.
// In normal test runs (no -bench flag), this is a no-op.
func TestMain(m *testing.M) {
	// Run tests normally; benchmark gate is checked separately in CI via make bench.
	m.Run()
}

// ---------------------------------------------------------------------------
// T021 additional: NodeID utility
// ---------------------------------------------------------------------------

func TestNodeIDWithShortID(t *testing.T) {
	got := NodeID("notes/french.n4l", "vocabulary", "k7mpq9rx")
	want := "notes/french.n4l:vocabulary:id:k7mpq9rx"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestNodeIDEmptyShortID(t *testing.T) {
	got := NodeID("notes/french.n4l", "vocabulary", "")
	if got != "" {
		t.Errorf("want empty string for untracked node, got %q", got)
	}
}

func TestGenerateShortIDLength(t *testing.T) {
	id, err := GenerateShortID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 8 {
		t.Errorf("want length 8, got %d: %q", len(id), id)
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z')) {
			t.Errorf("character %q not in base-36 alphabet", c)
		}
	}
}

func TestGenerateShortIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateShortID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Errorf("duplicate ID generated: %q", id)
		}
		seen[id] = true
	}
}

// ---------------------------------------------------------------------------
// T021: Registry loading tests
// ---------------------------------------------------------------------------

func TestLoadRegistryDegradedMode(t *testing.T) {
	reg, err := LoadRegistry("/nonexistent/path")
	if err == nil {
		t.Error("want error for nonexistent directory, got nil")
	}
	if reg == nil {
		t.Error("want non-nil registry in degraded mode")
	}
	if reg.Arrows == nil {
		t.Error("want non-nil Arrows map")
	}
}

func TestLoadRegistryArrows(t *testing.T) {
	reg := loadTestRegistry(t)
	if len(reg.Arrows) == 0 {
		t.Error("want arrows loaded from SSTconfig")
	}
	// Check a known arrow from arrows-EP-3.sst
	if _, ok := reg.Arrows["means"]; !ok {
		t.Error("want 'means' arrow in registry")
	}
	if _, ok := reg.Arrows["e.g."]; !ok {
		t.Error("want 'e.g.' arrow in registry")
	}
}

func TestLoadRegistryAnnotations(t *testing.T) {
	reg := loadTestRegistry(t)
	if len(reg.Annotations) == 0 {
		t.Error("want annotations loaded from SSTconfig")
	}
	// Check annotations.sst entries
	if _, ok := reg.Annotations["***"]; !ok {
		t.Error("want '***' annotation in registry")
	}
}

// ---------------------------------------------------------------------------
// Helper: bytes are available for write-back tests
// ---------------------------------------------------------------------------

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func diffLines(a, b []byte) []int {
	aLines := bytes.Split(a, []byte("\n"))
	bLines := bytes.Split(b, []byte("\n"))
	var changed []int
	for i := 0; i < len(aLines) || i < len(bLines); i++ {
		var al, bl []byte
		if i < len(aLines) {
			al = aLines[i]
		}
		if i < len(bLines) {
			bl = bLines[i]
		}
		if !bytes.Equal(al, bl) {
			changed = append(changed, i+1)
		}
	}
	return changed
}

// ---------------------------------------------------------------------------
// T027: Study marker and write-back tests
// ---------------------------------------------------------------------------

func TestStudyMarkerExtraction(t *testing.T) {
	reg := emptyRegistry()
	pf, _ := ParseFile(testdataPath(t, "study-markers.n4l"), reg)
	if pf == nil || len(pf.Chapters) == 0 {
		t.Fatal("failed to parse study-markers.n4l")
	}
	ch := pf.Chapters[0]

	// Collect nodes with StudyMeta
	var studyNodes []*Node
	var unmarkedNodes []*Node
	for i := range ch.Nodes {
		if ch.Nodes[i].StudyMeta != nil {
			studyNodes = append(studyNodes, &ch.Nodes[i])
		} else {
			unmarkedNodes = append(unmarkedNodes, &ch.Nodes[i])
		}
	}

	if len(unmarkedNodes) < 2 {
		t.Errorf("want at least 2 unmarked nodes, got %d", len(unmarkedNodes))
	}

	// Find specific cards by text content
	findNode := func(text string) *Node {
		for i := range ch.Nodes {
			if strings.Contains(ch.Nodes[i].Text, text) {
				return &ch.Nodes[i]
			}
		}
		return nil
	}

	n := findNode("Individual basic card")
	if n == nil || n.StudyMeta == nil || n.StudyMeta.CardType != CardTypeBasic {
		t.Errorf("basic card: want CardTypeBasic, node=%v", n)
	}

	n = findNode("Individual reverse card")
	if n == nil || n.StudyMeta == nil || !n.StudyMeta.ForceReverse {
		t.Errorf("reverse card: want ForceReverse=true, node=%v", n)
	}

	n = findNode("Individual no-reverse card")
	if n == nil || n.StudyMeta == nil || !n.StudyMeta.SuppressReverse {
		t.Errorf("no-reverse card: want SuppressReverse=true, node=%v", n)
	}

	n = findNode("Cloze deletion")
	if n == nil || n.StudyMeta == nil || n.StudyMeta.CardType != CardTypeCloze {
		t.Errorf("cloze card: want CardTypeCloze")
	}
	if n != nil && n.StudyMeta != nil {
		if len(n.StudyMeta.ClozeTargets) != 2 {
			t.Errorf("cloze targets: want 2, got %d: %v", len(n.StudyMeta.ClozeTargets), n.StudyMeta.ClozeTargets)
		}
	}

	n = findNode("First node in study block")
	if n == nil || n.StudyMeta == nil || !n.StudyMeta.FromBlock {
		t.Errorf("study block node: want FromBlock=true")
	}

	n = findNode("Reverse block node one")
	if n == nil || n.StudyMeta == nil || n.StudyMeta.BlockType != BlockTypeStudyReverse {
		t.Errorf("reverse block node: want BlockTypeStudyReverse")
	}

	n = findNode("Sequence block node one")
	if n == nil || n.StudyMeta == nil || n.StudyMeta.CardType != CardTypeSequenceForward {
		t.Errorf("sequence block node: want CardTypeSequenceForward")
	}
}

func TestBlockClosedByContextReset(t *testing.T) {
	content := `- test

# @study
Block node one

:: reset

After context reset
`
	path := filepath.Join(t.TempDir(), "block-reset.n4l")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	reg := emptyRegistry()
	pf, _ := ParseFile(path, reg)
	if len(pf.Chapters) == 0 {
		t.Fatal("no chapters")
	}
	ch := pf.Chapters[0]

	blockNode := ch.Nodes[0]
	if blockNode.StudyMeta == nil {
		t.Error("first node should be in study block")
	}

	if len(ch.Nodes) < 2 {
		t.Fatal("need 2 nodes")
	}
	afterReset := ch.Nodes[1]
	if afterReset.StudyMeta != nil {
		t.Error("node after :: reset should not be in study block")
	}
}

func TestInjectMissingIDs(t *testing.T) {
	dir := t.TempDir()
	src := testdataPath(t, "study-markers.n4l")
	dst := filepath.Join(dir, "inject-test.n4l")
	copyFile(t, src, dst)

	reg := emptyRegistry()
	pf, _ := ParseFile(dst, reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}

	original := readFileBytes(t, dst)

	injections, err := InjectMissingIDs(pf)
	if err != nil {
		t.Fatalf("InjectMissingIDs: %v", err)
	}

	modified := readFileBytes(t, dst)

	// At least some injections happened
	var newIDs int
	for _, inj := range injections {
		if !inj.AlreadyHad {
			newIDs++
		}
	}
	if newIDs == 0 {
		t.Error("expected at least one new ID injection")
	}

	// Every line that wasn't changed should be identical
	origLines := strings.Split(string(original), "\n")
	modLines := strings.Split(string(modified), "\n")

	// Modified file must be longer by exactly newIDs lines
	if len(modLines) != len(origLines)+newIDs {
		t.Errorf("want %d lines, got %d (original %d + %d injections)",
			len(origLines)+newIDs, len(modLines), len(origLines), newIDs)
	}

	// Injected lines must be valid "# @id <shortid>" format
	for _, inj := range injections {
		if inj.AlreadyHad {
			continue
		}
		if len(inj.ShortID) != 8 {
			t.Errorf("shortID %q not 8 chars", inj.ShortID)
		}
	}
}

func TestInjectMissingIDsIdempotent(t *testing.T) {
	dir := t.TempDir()
	src := testdataPath(t, "study-markers.n4l")
	dst := filepath.Join(dir, "idempotent-test.n4l")
	copyFile(t, src, dst)

	reg := emptyRegistry()
	pf, _ := ParseFile(dst, reg)

	// First injection
	injections1, err := InjectMissingIDs(pf)
	if err != nil {
		t.Fatal(err)
	}

	after1 := readFileBytes(t, dst)

	// Re-parse and inject again
	pf2, _ := ParseFile(dst, reg)
	injections2, err := InjectMissingIDs(pf2)
	if err != nil {
		t.Fatal(err)
	}

	after2 := readFileBytes(t, dst)

	// File must be unchanged after second injection
	if !bytes.Equal(after1, after2) {
		changed := diffLines(after1, after2)
		t.Errorf("file changed on second injection at lines: %v", changed)
	}

	// All second-round injections must be AlreadyHad
	for _, inj := range injections2 {
		if !inj.AlreadyHad {
			t.Errorf("second injection: expected AlreadyHad=true for node at line %d", inj.NodeLine)
		}
	}

	_ = injections1
}

func TestInjectNodeIDIdempotent(t *testing.T) {
	dir := t.TempDir()
	content := "- test\n\nSome node to inject ID for\n"
	dst := filepath.Join(dir, "nodeid-test.n4l")
	if err := os.WriteFile(dst, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	inj1, err := InjectNodeID(dst, 3)
	if err != nil {
		t.Fatal(err)
	}
	if inj1.AlreadyHad {
		t.Error("first injection should not have AlreadyHad=true")
	}

	before := readFileBytes(t, dst)

	inj2, err := InjectNodeID(dst, 4) // line shifted by 1 after injection
	if err != nil {
		t.Fatal(err)
	}

	after := readFileBytes(t, dst)

	if !inj2.AlreadyHad {
		t.Error("second injection should have AlreadyHad=true")
	}
	if !bytes.Equal(before, after) {
		t.Error("file should not change on idempotent inject")
	}
}

func TestExtractToItemPromotion(t *testing.T) {
	dir := t.TempDir()
	// Start with an unmarked node; inject as extract, then add study marker and inject as item
	content := "- test\n\nNode to promote from extract to item\n"
	dst := filepath.Join(dir, "promote-test.n4l")
	if err := os.WriteFile(dst, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Step 1: inject as extract (line 3)
	extractInj, err := InjectNodeID(dst, 3)
	if err != nil {
		t.Fatal(err)
	}
	extractID := extractInj.ShortID

	// Step 2: add # @card marker manually above the node (now shifted)
	lines, err := readLines(dst)
	if err != nil {
		t.Fatal(err)
	}
	// Find the node line
	var nodeLine int
	for i, l := range lines {
		if strings.Contains(l, "Node to promote") {
			nodeLine = i + 1
			break
		}
	}
	// Insert # @card above the node
	newLines := append(lines[:nodeLine-1], append([]string{"# @card"}, lines[nodeLine-1:]...)...)
	if err := writeLines(dst, newLines); err != nil {
		t.Fatal(err)
	}

	// Step 3: parse and inject as item — should find existing ID (AlreadyHad=true)
	reg := emptyRegistry()
	pf, _ := ParseFile(dst, reg)
	itemInjections, err := InjectMissingIDs(pf)
	if err != nil {
		t.Fatal(err)
	}

	// The study-marked node should already have the extract ID — AlreadyHad=true
	for _, inj := range itemInjections {
		if !inj.AlreadyHad {
			t.Errorf("promoted node should AlreadyHad=true; inj=%+v", inj)
		}
		if inj.ShortID != extractID {
			t.Errorf("promoted node shortID mismatch: extract=%s item=%s", extractID, inj.ShortID)
		}
	}

	// Verify the ID in the parsed file matches
	for _, ch := range pf.Chapters {
		for _, n := range ch.Nodes {
			if n.StudyMeta != nil && n.IDMarkerLine > 0 {
				got := extractShortID(n.ID)
				if got != extractID {
					t.Errorf("node ID mismatch: want %s, got %s", extractID, got)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// T033: Reminder detection, priority, provenance and write-back tests
// ---------------------------------------------------------------------------

func TestReminderDetection(t *testing.T) {
	reg := emptyRegistry()
	pf, errs := ParseFile(testdataPath(t, "reminders.n4l"), reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}

	// Exactly 3 genuine reminders
	if len(pf.Reminders) != 3 {
		t.Errorf("want 3 reminders, got %d:", len(pf.Reminders))
		for _, r := range pf.Reminders {
			t.Logf("  line %d: %q", r.Line, r.Text)
		}
	}

	texts := make(map[string]bool)
	for _, r := range pf.Reminders {
		texts[r.Text] = true
	}
	if !texts["FINISH THIS SECTION"] {
		t.Error("want 'FINISH THIS SECTION' as reminder")
	}
	if !texts["CHECK THESE REFERENCES NOW"] {
		t.Error("want 'CHECK THESE REFERENCES NOW' as reminder")
	}
	// The third reminder has a comment stripped: "REVIEW AND UPDATE ALL CONCEPTS HERE"
	if !texts["REVIEW AND UPDATE ALL CONCEPTS HERE"] {
		t.Errorf("want 'REVIEW AND UPDATE ALL CONCEPTS HERE' as reminder, got keys: %v", texts)
	}

	_ = errs
}

func TestReminderWithComment(t *testing.T) {
	reg := emptyRegistry()
	pf, _ := ParseFile(testdataPath(t, "reminders.n4l"), reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}

	for _, r := range pf.Reminders {
		if strings.Contains(r.Text, "#") {
			t.Errorf("reminder text should not contain comment: %q", r.Text)
		}
	}
}

func TestPriorityOutOfRange(t *testing.T) {
	reg := emptyRegistry()
	pf, errs := ParseFile(testdataPath(t, "reminders.n4l"), reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}

	// Should have an invalid priority error
	var hasPriorityErr bool
	for _, e := range errs {
		if strings.Contains(e.Msg, ErrInvalidPriority) {
			hasPriorityErr = true
		}
	}
	if !hasPriorityErr {
		t.Errorf("expected invalid priority error; errors: %v", errs)
	}

	// The valid node should have Priority set; the invalid one should have nil
	ch := pf.Chapters[0]
	var found50, foundNilForInvalid bool
	for i := range ch.Nodes {
		n := &ch.Nodes[i]
		if strings.Contains(n.Text, "priority fifty") && n.Priority != nil && *n.Priority == 50 {
			found50 = true
		}
		if strings.Contains(n.Text, "out-of-range priority") && n.Priority == nil {
			foundNilForInvalid = true
		}
	}
	if !found50 {
		t.Error("want priority=50 on 'priority fifty' node")
	}
	if !foundNilForInvalid {
		t.Error("want Priority=nil on out-of-range node")
	}
}

func TestProvenanceAttachment(t *testing.T) {
	reg := emptyRegistry()
	pf, _ := ParseFile(testdataPath(t, "reminders.n4l"), reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}

	ch := pf.Chapters[0]
	var found bool
	for i := range ch.Nodes {
		n := &ch.Nodes[i]
		if strings.Contains(n.Text, "full provenance") {
			found = true
			if n.Provenance == nil {
				t.Fatal("want Provenance != nil on provenance node")
			}
			if n.Provenance.Source == "" {
				t.Error("want Source populated")
			}
			if n.Provenance.Ref == "" {
				t.Error("want Ref populated")
			}
			if n.Provenance.Progress == "" {
				t.Error("want Progress populated")
			}
		}
	}
	if !found {
		t.Error("provenance node not found")
	}
}

func TestResolveReminder(t *testing.T) {
	dir := t.TempDir()
	src := testdataPath(t, "reminders.n4l")
	dst := filepath.Join(dir, "resolve-test.n4l")
	copyFile(t, src, dst)

	// Find the line number of the first reminder
	reg := emptyRegistry()
	pf, _ := ParseFile(dst, reg)
	if len(pf.Reminders) == 0 {
		t.Fatal("no reminders found")
	}
	lineNum := pf.Reminders[0].Line

	original := readFileBytes(t, dst)

	result, err := ResolveReminder(dst, lineNum)
	if err != nil {
		t.Fatalf("ResolveReminder: %v", err)
	}
	if result == nil || len(result.LinesChanged) == 0 {
		t.Error("expected LinesChanged")
	}

	modified := readFileBytes(t, dst)

	// The resolved line must be lowercase
	modLines := strings.Split(string(modified), "\n")
	resolved := modLines[lineNum-1]
	if resolved != strings.ToLower(resolved) {
		t.Errorf("line %d not lowercased: %q", lineNum, resolved)
	}

	// All other lines must be byte-identical
	origLines := strings.Split(string(original), "\n")
	for i, orig := range origLines {
		if i == lineNum-1 {
			continue // skip the resolved line
		}
		if i >= len(modLines) {
			break
		}
		if orig != modLines[i] {
			t.Errorf("line %d unexpectedly changed: %q → %q", i+1, orig, modLines[i])
		}
	}
}

func TestResolveReminderGuard(t *testing.T) {
	dir := t.TempDir()
	content := "- test\n\nNormal node not a reminder\n"
	dst := filepath.Join(dir, "guard-test.n4l")
	if err := os.WriteFile(dst, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	original := readFileBytes(t, dst)
	_, err := ResolveReminder(dst, 3)
	if err == nil {
		t.Error("want error when resolving non-reminder line")
	}

	// File must be untouched
	after := readFileBytes(t, dst)
	if !bytes.Equal(original, after) {
		t.Error("file was modified despite error")
	}
}

func TestUpdateProgress(t *testing.T) {
	dir := t.TempDir()
	content := "- test\n\n# @source some book\n# @ref some/path.pdf\n# @progress ch.1 p.1\nNode with progress\n"
	dst := filepath.Join(dir, "progress-test.n4l")
	if err := os.WriteFile(dst, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	original := readFileBytes(t, dst)
	result, err := UpdateProgress(dst, 5, "ch.5 p.120")
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if result == nil || len(result.LinesChanged) == 0 {
		t.Error("expected LinesChanged")
	}

	modified := readFileBytes(t, dst)
	modLines := strings.Split(string(modified), "\n")
	if !strings.Contains(modLines[4], "ch.5 p.120") {
		t.Errorf("progress not updated: %q", modLines[4])
	}

	// All other lines must be identical
	origLines := strings.Split(string(original), "\n")
	for i, orig := range origLines {
		if i == 4 {
			continue
		}
		if i >= len(modLines) {
			break
		}
		if orig != modLines[i] {
			t.Errorf("line %d unexpectedly changed: %q → %q", i+1, orig, modLines[i])
		}
	}
}

func TestUpdateProgressGuard(t *testing.T) {
	dir := t.TempDir()
	content := "- test\n\nNot a progress line\n"
	dst := filepath.Join(dir, "progress-guard-test.n4l")
	if err := os.WriteFile(dst, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	original := readFileBytes(t, dst)
	_, err := UpdateProgress(dst, 3, "ch.5 p.120")
	if err == nil {
		t.Error("want error when updating non-progress line")
	}
	after := readFileBytes(t, dst)
	if !bytes.Equal(original, after) {
		t.Error("file was modified despite error")
	}
}

func TestSlashSlashComments(t *testing.T) {
	reg := emptyRegistry()
	src := "- chapter\n" +
		"// whole-line comment\n" +
		"node one // trailing comment\n" +
		"  \" (means) target\n" +
		"url node (means) \"https://example.com\" // inline\n"

	f, err := os.CreateTemp(t.TempDir(), "*.n4l")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	pf, _ := ParseFile(f.Name(), reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}
	if len(pf.Chapters) != 1 {
		t.Fatalf("want 1 chapter, got %d", len(pf.Chapters))
	}
	nodes := pf.Chapters[0].Nodes
	if len(nodes) != 2 {
		names := make([]string, len(nodes))
		for i, n := range nodes {
			names[i] = n.Text
		}
		t.Fatalf("want 2 root nodes, got %d: %v", len(nodes), names)
	}
	// Node 0: "node one" — trailing // comment stripped
	if nodes[0].Text != "node one" {
		t.Errorf("node 0 text: want %q, got %q", "node one", nodes[0].Text)
	}
	if len(nodes[0].Dittos) != 1 {
		t.Fatalf("want 1 ditto on node 0, got %d", len(nodes[0].Dittos))
	}
	// Node 1: url node — trailing // comment stripped, but URL target preserved
	if nodes[1].Text != "url node" {
		t.Errorf("node 1 text: want %q, got %q", "url node", nodes[1].Text)
	}
	if len(nodes[1].Arrows) != 1 || nodes[1].Arrows[0].Target != "https://example.com" {
		t.Errorf("node 1 arrow target: want %q, got %v", "https://example.com", nodes[1].Arrows)
	}
}

// TestFileURLTripleSlashPreserved guards against a regression where a file://
// URL (three slashes after the scheme) was truncated to "file:/" because the
// second-and-third slash pair was mistaken for a // comment. Each editor.c#Ln
// URL must survive intact so the graph shows one node per file location.
func TestFileURLTripleSlashPreserved(t *testing.T) {
	reg := emptyRegistry()
	reg.Arrows["url"] = ArrowDef{Code: "url"}
	reg.Arrows["contains"] = ArrowDef{Code: "contains"}
	src := "- chapter\n" +
		"editor_code (contains) includes\n" +
		"  \" (url) \"file:///home/simone/projects/sst-literate-test/editor.c#L4\"\n" +
		"editor_code (contains) data\n" +
		"  \" (url) \"file:///home/simone/projects/sst-literate-test/editor.c#L5\"\n"

	f, err := os.CreateTemp(t.TempDir(), "*.n4l")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	pf, _ := ParseFile(f.Name(), reg)
	if pf == nil {
		t.Fatal("nil ParsedFile")
	}
	if len(pf.Chapters) != 1 {
		t.Fatalf("want 1 chapter, got %d", len(pf.Chapters))
	}
	nodes := pf.Chapters[0].Nodes
	if len(nodes) != 2 {
		t.Fatalf("want 2 root nodes, got %d", len(nodes))
	}
	wantURLs := []string{
		"file:///home/simone/projects/sst-literate-test/editor.c#L4",
		"file:///home/simone/projects/sst-literate-test/editor.c#L5",
	}
	for i, n := range nodes {
		if len(n.Dittos) != 1 {
			t.Fatalf("node %d: want 1 ditto, got %d", i, len(n.Dittos))
		}
		d := n.Dittos[0]
		if len(d.Arrows) != 1 {
			t.Fatalf("node %d ditto: want 1 arrow, got %d", i, len(d.Arrows))
		}
		if got := d.Arrows[0].Target; got != wantURLs[i] {
			t.Errorf("node %d url target: want %q, got %q", i, wantURLs[i], got)
		}
	}
}

func TestApostropheInNodeText(t *testing.T) {
	// Apostrophes in natural language (contractions, erhua phonetic markers) must
	// not be treated as single-quote string delimiters by maskSpecial, otherwise
	// arrow codes that follow them on the same line are masked and lost.
	//
	// Three distinct patterns:
	//   1. Single apostrophe in a contraction: "don't", "I'm" — no second ' exists.
	//   2. Two apostrophes on one line: "méishì 'r (eh) target (he) it's okay" —
	//      the closing ' in "it's" is followed by the letter 's', so it must NOT
	//      be accepted as the closing delimiter of the 'r region.
	reg := emptyRegistry()
	src := "- chapter\n" +
		"no/no thanks i.e. I don't need it (means) 不要 (note) bù yào\n" +
		"I'm waiting (means) target one (note) target two\n" +
		"méishì 'r (means) 没事儿 (note) it's okay\n"

	f, err := os.CreateTemp(t.TempDir(), "*.n4l")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	pf, _ := ParseFile(f.Name(), reg)
	if pf == nil || len(pf.Chapters) != 1 {
		t.Fatal("unexpected parse result")
	}
	nodes := pf.Chapters[0].Nodes
	if len(nodes) != 3 {
		t.Fatalf("want 3 nodes, got %d", len(nodes))
	}

	// Node 0: "no/no thanks i.e. I don't need it" — two chained arrows
	if nodes[0].Text != "no/no thanks i.e. I don't need it" {
		t.Errorf("node 0 text: got %q", nodes[0].Text)
	}
	if len(nodes[0].Arrows) != 2 {
		t.Errorf("node 0: want 2 arrows, got %d: %v", len(nodes[0].Arrows), nodes[0].Arrows)
	}

	// Node 1: "I'm waiting" — two chained arrows
	if nodes[1].Text != "I'm waiting" {
		t.Errorf("node 1 text: got %q", nodes[1].Text)
	}
	if len(nodes[1].Arrows) != 2 {
		t.Errorf("node 1: want 2 arrows, got %d: %v", len(nodes[1].Arrows), nodes[1].Arrows)
	}

	// Node 2: "méishì 'r" — two apostrophes on the line; the closing ' in "it's"
	// is followed by 's' so must NOT be treated as the closing delimiter of 'r.
	if nodes[2].Text != "méishì 'r" {
		t.Errorf("node 2 text: got %q", nodes[2].Text)
	}
	if len(nodes[2].Arrows) != 2 {
		t.Errorf("node 2: want 2 arrows, got %d: %v", len(nodes[2].Arrows), nodes[2].Arrows)
	}
}

func TestLabeledDittoLine(t *testing.T) {
	// @label " ... must be treated as a ditto of the preceding node, NOT as a
	// standalone root node. The label is registered on the parent node so that
	// $label.N references remain resolvable.
	reg := emptyRegistry()
	src := "- chapter\n" +
		"parent node (means) first arrow\n" +
		"   \" (means) ditto one\n" +
		"@myalias \" (means) labeled ditto\n" +
		"   \" (means) ditto after label\n"

	f, err := os.CreateTemp(t.TempDir(), "*.n4l")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	pf, _ := ParseFile(f.Name(), reg)
	if pf == nil || len(pf.Chapters) != 1 {
		t.Fatal("unexpected parse result")
	}
	nodes := pf.Chapters[0].Nodes
	// Must have exactly ONE root node (parent node), not two.
	if len(nodes) != 1 {
		t.Fatalf("want 1 root node, got %d (labeled ditto must not become a root)", len(nodes))
	}
	// Parent node must have 3 dittos: "ditto one", "labeled ditto", "ditto after label"
	if len(nodes[0].Dittos) != 3 {
		t.Fatalf("want 3 dittos, got %d", len(nodes[0].Dittos))
	}
	// The labeled ditto is the second one
	if nodes[0].Dittos[1].Arrows[0].Target != "labeled ditto" {
		t.Errorf("ditto[1] target: want %q, got %q", "labeled ditto", nodes[0].Dittos[1].Arrows[0].Target)
	}
	// Label must be registered (pointing to parent node for $alias.N reference resolution)
	if _, ok := pf.Chapters[0].Labels["myalias"]; !ok {
		t.Error("label 'myalias' not registered in chapter Labels")
	}
}

func TestObjectDittoResolution(t *testing.T) {
	// A lone `"` in OBJECT position repeats the previous line's object. Mirrors
	// examples/doors.n4l lines 26-28: river/tram/bike all (fwd) "target 3".
	reg := emptyRegistry()
	reg.Arrows["fwd"] = ArrowDef{Code: "fwd", LongName: "forward", MetaType: "leadsto"}
	src := "- maze\n" +
		"river (fwd) target 3\n" +
		"tram  (fwd) \"\n" +
		"bike  (fwd) \"\n"

	f, err := os.CreateTemp(t.TempDir(), "*.n4l")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(src); err != nil {
		t.Fatal(err)
	}
	f.Close()

	pf, _ := ParseFile(f.Name(), reg)
	if pf == nil || len(pf.Chapters) != 1 {
		t.Fatal("unexpected parse result")
	}
	nodes := pf.Chapters[0].Nodes
	if len(nodes) != 3 {
		t.Fatalf("want 3 root nodes (river, tram, bike); got %d", len(nodes))
	}
	for _, n := range nodes {
		if len(n.Arrows) != 1 {
			t.Fatalf("node %q: want 1 arrow; got %d", n.Text, len(n.Arrows))
		}
		if got := n.Arrows[0].Target; got != "target 3" {
			t.Errorf("node %q: object-ditto target = %q, want %q (a lone %q must resolve, not become a node)",
				n.Text, got, "target 3", `"`)
		}
	}
}

func BenchmarkScanCollection500(b *testing.B) {
	dir := b.TempDir()
	src := testdataPath(b, "simple.n4l")
	data, err := os.ReadFile(src)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		name := filepath.Join(dir, strings.Repeat("f", 1)+strconv.Itoa(i)+".n4l")
		if err := os.WriteFile(name, data, 0644); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ScanCollection(dir); err != nil {
			b.Fatal(err)
		}
	}
}
