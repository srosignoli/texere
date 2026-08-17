package collectionpath

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveAcceptsPathsUnderRoot(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "reading", "x.n4l"), "x")
	got, err := Resolve(root, "reading/x.n4l")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := filepath.Join(root, "reading", "x.n4l"); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestResolveAcceptsAnAbsolutePathAlreadyUnderRoot(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "a.n4l")
	mustWrite(t, abs, "a")
	got, err := Resolve(root, abs)
	if err != nil || got != abs {
		t.Fatalf("Resolve(%q) = %q, %v", abs, got, err)
	}
}

// The stale-database case: an absolute path from the collection's old location.
func TestResolveRefusesAnAbsolutePathElsewhere(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(t.TempDir(), "elsewhere.n4l")
	mustWrite(t, other, "e")
	if _, err := Resolve(root, other); err == nil {
		t.Fatal("Resolve accepted a path outside the collection")
	}
}

func TestResolveRefusesTraversal(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../escape.n4l", "reading/../../escape.n4l", "reading/../.."} {
		if _, err := Resolve(root, rel); err == nil {
			t.Errorf("Resolve(%q) accepted a traversal", rel)
		}
	}
}

// A filename may contain dots without being a traversal.
func TestResolveAcceptsDotsInsideAName(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a..b.n4l"), "x")
	if _, err := Resolve(root, "a..b.n4l"); err != nil {
		t.Errorf("Resolve rejected a legitimate name: %v", err)
	}
}

// The strengthening over the lexical helper this replaces: a symlink inside the
// collection pointing outside it must be refused, not followed.
func TestResolveRefusesASymlinkEscapingTheCollection(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.n4l")
	mustWrite(t, outside, "s")
	link := filepath.Join(root, "link.n4l")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Resolve(root, "link.n4l"); err == nil {
		t.Fatal("Resolve followed a symlink out of the collection")
	}
}

// A symlink that stays inside is fine — the guard must not reject all links.
func TestResolveAcceptsASymlinkInsideTheCollection(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.n4l")
	mustWrite(t, target, "r")
	link := filepath.Join(root, "alias.n4l")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Resolve(root, "alias.n4l"); err != nil {
		t.Errorf("Resolve rejected an internal symlink: %v", err)
	}
}

// A file being created does not exist yet, so EvalSymlinks cannot be applied to
// it — the parent must be evaluated instead. Without this, creating any new note
// would fail.
func TestResolveAcceptsAPathThatDoesNotExistYet(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reading"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "reading/brand-new.n4l"); err != nil {
		t.Errorf("Resolve rejected a not-yet-created file: %v", err)
	}
}

// ...but not if its parent escapes.
func TestResolveRefusesANewFileUnderAnEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(root, "away")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Resolve(root, "away/new.n4l"); err == nil {
		t.Fatal("Resolve accepted a new file under an escaping symlink")
	}
}

func TestEscapeErrorNamesThePath(t *testing.T) {
	root := t.TempDir()
	_, err := Resolve(root, "../escape.n4l")
	var esc *EscapeError
	if !errors.As(err, &esc) {
		t.Fatalf("want *EscapeError, got %T", err)
	}
	if !strings.Contains(esc.Error(), "escape.n4l") {
		t.Errorf("error does not name the file: %q", esc.Error())
	}
}

// Gap found while writing Control 2 (a bare strings.HasPrefix(full, root) in
// place of the separator-aware check): a sibling directory whose name has the
// root's name as a string prefix must not be treated as contained. No other
// test in this file exercises that shape — t.TempDir() roots don't collide
// this way by construction — so this one builds it deliberately.
func TestResolveRefusesASiblingDirectoryWithASharedNamePrefix(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "x")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(base, "xy", "f.n4l")
	mustWrite(t, sibling, "f")
	if _, err := Resolve(root, sibling); err == nil {
		t.Fatalf("Resolve(%q, %q) accepted a sibling sharing the root's name as a prefix", root, sibling)
	}
}

// Found in review: under() built its prefix as root+separator unconditionally.
// When root is already just a separator ("/" on Unix, and plausibly "D:\" on
// Windows), that produces "//", which matches nothing — every path under a
// filesystem-root collection was refused. It failed closed, not open, but a
// collection opened at a filesystem root couldn't be used at all. Exercised
// directly against under() rather than through Resolve: a real filesystem
// root isn't something a test can safely populate with fixtures.
func TestUnderTreatsAFilesystemRootAsContainingEverythingBeneathIt(t *testing.T) {
	root := string(filepath.Separator)
	full := filepath.Join(root, "etc", "passwd")
	if !under(full, root) {
		t.Errorf("under(%q, %q) = false, want true — root is the whole filesystem", full, root)
	}
}

// The ancestor walk was only ever exercised one level deep (brand-new.n4l
// under an existing "reading"). Found in review: nothing shipped covered
// several consecutive missing directories, even though the recursion is
// written to handle it.
func TestResolveAcceptsAPathSeveralMissingDirectoriesDeep(t *testing.T) {
	root := t.TempDir()
	if _, err := Resolve(root, "a/b/c/brand-new.n4l"); err != nil {
		t.Errorf("Resolve rejected a path several missing directories deep: %v", err)
	}
}

// Same shape as TestResolveAcceptsDotsInsideAName but in a directory
// component rather than the leaf name — the exact case the deleted
// media white-box test covered ("a..b/c.png").
func TestResolveAcceptsDotsInsideADirectoryName(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a..b", "c.png"), "x")
	if _, err := Resolve(root, "a..b/c.png"); err != nil {
		t.Errorf("Resolve rejected a legitimate directory name containing dots: %v", err)
	}
}
