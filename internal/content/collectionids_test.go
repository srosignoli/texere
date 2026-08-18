package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCollectionFile writes body to <dir>/<rel>, creating intermediate directories.
func writeCollectionFile(t *testing.T, dir, rel, body string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// scriptShortIDs makes issuance deterministic for the duration of one test by
// replacing the package's short-ID source with a fixed sequence. Restores the
// real generator on cleanup. Tests using it must not call t.Parallel.
func scriptShortIDs(t *testing.T, ids ...string) {
	t.Helper()
	saved := newShortID
	i := 0
	newShortID = func() (string, error) {
		if i >= len(ids) {
			t.Fatalf("scripted generator exhausted after %d ids; the code asked for more", len(ids))
		}
		id := ids[i]
		i++
		return id, nil
	}
	t.Cleanup(func() { newShortID = saved })
}

func TestCollectionShortIDsFindsMarkersInNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	writeCollectionFile(t, dir, "top.n4l", "- top\n\n# @id aaaaaaaa\nA node\n")
	writeCollectionFile(t, dir, "sub/mid.n4l", "- mid\n\n# @id bbbbbbbb\nB node\n")
	writeCollectionFile(t, dir, "sub/deeper/leaf.n4l", "- leaf\n\n# @id cccccccc\nC node\n")

	got, err := CollectionShortIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"aaaaaaaa", "bbbbbbbb", "cccccccc"} {
		if !got[want] {
			t.Errorf("CollectionShortIDs missing %q; got %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("CollectionShortIDs returned %d ids, want 3: %v", len(got), got)
	}
}

func TestCollectionShortIDsIgnoresNonN4LFiles(t *testing.T) {
	dir := t.TempDir()
	writeCollectionFile(t, dir, "notes.n4l", "- notes\n\n# @id aaaaaaaa\nA node\n")
	writeCollectionFile(t, dir, "readme.md", "# @id ffffffff\n")
	writeCollectionFile(t, dir, "notes.n4l.bak", "# @id eeeeeeee\n")

	got, err := CollectionShortIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["ffffffff"] || got["eeeeeeee"] {
		t.Errorf("CollectionShortIDs read a non-.n4l file: %v", got)
	}
	if !got["aaaaaaaa"] {
		t.Errorf("CollectionShortIDs missed the .n4l marker: %v", got)
	}
}

// The reserved app directories hold machine-written data, not authored notes;
// scanning them would be wasted I/O at best. isExcludedDir already encodes the
// rule for ScanCollection and the watcher — CollectionShortIDs shares it.
func TestCollectionShortIDsSkipsExcludedDirectories(t *testing.T) {
	dir := t.TempDir()
	writeCollectionFile(t, dir, "notes.n4l", "- notes\n\n# @id aaaaaaaa\nA node\n")
	writeCollectionFile(t, dir, "sst_data/stale.n4l", "# @id dddddddd\n")
	writeCollectionFile(t, dir, ".hidden/stale.n4l", "# @id 99999999\n")

	got, err := CollectionShortIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["dddddddd"] {
		t.Error("CollectionShortIDs descended into sst_data")
	}
	if got["99999999"] {
		t.Error("CollectionShortIDs descended into a hidden directory")
	}
}

func TestCollectionShortIDsOnAnEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := CollectionShortIDs(dir)
	if err != nil {
		t.Fatalf("empty directory must not be an error: %v", err)
	}
	if got == nil {
		t.Fatal("CollectionShortIDs returned a nil map; callers index into it directly")
	}
	if len(got) != 0 {
		t.Errorf("want no ids, got %v", got)
	}
}

func TestCollectionShortIDsOnAMissingDirectory(t *testing.T) {
	_, err := CollectionShortIDs(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("a missing collection root must be an error, not a silently empty set")
	}
}

// The point of the feature. "aaaaaaaa" is used by a DIFFERENT file in the
// collection, so nothing in the file being written names it — only the `taken`
// set does. The generator is scripted to offer "aaaaaaaa" first, so an injector
// that ignores `taken` writes it 100% of the time and this test fails 100% of
// the time. That determinism is the whole reason for the scripted generator: with
// the real random source the same bug would be invisible, because a collision is
// improbable rather than impossible.
func TestInjectionSkipsAnIDTakenElsewhereInTheCollection(t *testing.T) {
	dir := t.TempDir()
	writeCollectionFile(t, dir, "other.n4l", "- other\n\n# @id aaaaaaaa\nSomeone else's node\n")
	target := writeCollectionFile(t, dir, "target.n4l", "- target\n\n# @card\nA node needing an id\n")

	taken, err := CollectionShortIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !taken["aaaaaaaa"] {
		t.Fatal("setup: the id used by other.n4l was not collected")
	}

	scriptShortIDs(t, "aaaaaaaa", "bbbbbbbb")

	parsed, perrs := ParseFile(target, nil)
	if len(perrs) > 0 {
		t.Fatalf("parse errors: %v", perrs)
	}
	injections, err := InjectMissingIDs(parsed, taken)
	if err != nil {
		t.Fatal(err)
	}
	if len(injections) != 1 {
		t.Fatalf("want 1 injection, got %d: %+v", len(injections), injections)
	}
	if injections[0].ShortID != "bbbbbbbb" {
		t.Errorf("issued %q; want %q — the id taken by other.n4l must be skipped",
			injections[0].ShortID, "bbbbbbbb")
	}

	written := readFileBytes(t, target)
	if strings.Contains(string(written), "# @id aaaaaaaa") {
		t.Errorf("target.n4l was written the id already used by other.n4l:\n%s", written)
	}
}

// InjectNodeID takes the same guarantee; it is the single-node entry point used
// when an unmarked node enters the distillation pipeline.
func TestInjectNodeIDSkipsAnIDTakenElsewhereInTheCollection(t *testing.T) {
	dir := t.TempDir()
	writeCollectionFile(t, dir, "other.n4l", "- other\n\n# @id aaaaaaaa\nSomeone else's node\n")
	target := writeCollectionFile(t, dir, "target.n4l", "- target\n\nA node needing an id\n")

	taken, err := CollectionShortIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	scriptShortIDs(t, "aaaaaaaa", "bbbbbbbb")

	inj, err := InjectNodeID(target, 3, taken)
	if err != nil {
		t.Fatal(err)
	}
	if inj.ShortID != "bbbbbbbb" {
		t.Errorf("issued %q; want %q", inj.ShortID, "bbbbbbbb")
	}
}

// InjectMissingExtractIDs is the third injector; extracts share one id space
// with study items, so a taken item id must block an extract id too.
func TestInjectMissingExtractIDsSkipsAnIDTakenElsewhereInTheCollection(t *testing.T) {
	dir := t.TempDir()
	writeCollectionFile(t, dir, "other.n4l", "- other\n\n# @id aaaaaaaa\nSomeone else's node\n")
	target := writeCollectionFile(t, dir, "target.n4l",
		"- target\nanchoring shifts estimates\n    (from) \"sources/f.txt char=20\"\n")

	taken, err := CollectionShortIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	scriptShortIDs(t, "aaaaaaaa", "bbbbbbbb")

	reg := &SSTRegistry{Arrows: map[string]ArrowDef{"from": {Code: "from", LongName: "from"}}}
	parsed, perrs := ParseFile(target, reg)
	if len(perrs) > 0 {
		t.Fatalf("parse errors: %v", perrs)
	}
	injections, err := InjectMissingExtractIDs(parsed, taken)
	if err != nil {
		t.Fatal(err)
	}
	if len(injections) != 1 {
		t.Fatalf("want 1 injection, got %d: %+v", len(injections), injections)
	}
	if injections[0].ShortID != "bbbbbbbb" {
		t.Errorf("issued %q; want %q", injections[0].ShortID, "bbbbbbbb")
	}
}

// One Sync pass injects into many files. An id issued into the first file is on
// disk but was not in the set scanned before the pass began, so the injectors
// must feed what they issue back into `taken` or the second file can be given
// the same id.
func TestInjectionRecordsNewlyIssuedIDsInTaken(t *testing.T) {
	dir := t.TempDir()
	target := writeCollectionFile(t, dir, "target.n4l", "- target\n\n# @card\nA node needing an id\n")

	taken, err := CollectionShortIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	scriptShortIDs(t, "bbbbbbbb")

	parsed, perrs := ParseFile(target, nil)
	if len(perrs) > 0 {
		t.Fatalf("parse errors: %v", perrs)
	}
	if _, err := InjectMissingIDs(parsed, taken); err != nil {
		t.Fatal(err)
	}
	if !taken["bbbbbbbb"] {
		t.Error("the newly issued id was not recorded in taken; the next file in the same pass could be given it again")
	}
}
