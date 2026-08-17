// Package collectionpath resolves a path relative to a collection root,
// refusing anything — a stale absolute path, a "../" traversal, or a symlink —
// that would place the result outside the collection.
//
// This exists because the learning store used to persist absolute file paths.
// A copied or moved collection carried a database that still pointed at the
// original location, and callers trusted those paths without checking they
// still lay under the (possibly new) collection root. Resolve is the guard
// that makes that class of bug impossible, regardless of how the untrusted
// path arrived — from disk, from a URL, or from user input.
package collectionpath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EscapeError is returned when a path resolves outside the collection.
type EscapeError struct{ Path string }

func (e *EscapeError) Error() string {
	return fmt.Sprintf("path escapes the collection: %s", e.Path)
}

// Resolve joins rel to root and returns the absolute path, refusing anything
// that escapes root. rel may be relative or absolute; an absolute path is
// accepted only if it already lies under root.
//
// Symlinks are the reason this cannot be a lexical join-and-check: a name
// inside the collection may point anywhere on disk. Resolve evaluates
// symlinks on the target (or on its nearest existing ancestor, for a file
// about to be created) and on root itself — a collection reached through a
// symlinked parent must still compare equal — before deciding containment.
func Resolve(root, rel string) (string, error) {
	root = filepath.Clean(root)

	var full string
	if filepath.IsAbs(rel) {
		full = filepath.Clean(rel)
	} else {
		full = filepath.Clean(filepath.Join(root, rel))
	}

	// Fast lexical rejection before touching the filesystem: catches a stale
	// absolute path elsewhere and a "../" traversal without requiring the
	// target to exist.
	if !under(full, root) {
		return "", &EscapeError{Path: rel}
	}

	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("collectionpath: resolve root %q: %w", root, err)
	}

	evalFull, err := evalSymlinksOrAncestor(full)
	if err != nil {
		return "", fmt.Errorf("collectionpath: resolve %q: %w", full, err)
	}

	if !under(evalFull, evalRoot) {
		return "", &EscapeError{Path: rel}
	}

	return full, nil
}

// under reports whether full is root itself or sits under root plus a
// separator. The separator matters: without it, "/tmp/xy" would count as
// under "/tmp/x".
//
// root's own trailing separator, if it has one, is not duplicated before the
// comparison. Without that, a root that is already just a separator ("/" on
// Unix, plausibly "D:\" on Windows — the whole filesystem, or a symlink chain
// that bottoms out there) would build "//" and match nothing, refusing every
// path under it.
func under(full, root string) bool {
	if full == root {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(full, prefix)
}

// evalSymlinksOrAncestor evaluates symlinks in path. When path does not exist
// yet — a file about to be created has nothing on disk to evaluate — it walks
// up to the nearest existing ancestor, evaluates that, and re-appends the
// missing suffix lexically.
func evalSymlinksOrAncestor(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	parent := filepath.Dir(path)
	if parent == path {
		// Reached the top of the filesystem without finding anything real.
		return "", err
	}
	evalParent, parentErr := evalSymlinksOrAncestor(parent)
	if parentErr != nil {
		return "", parentErr
	}
	return filepath.Join(evalParent, filepath.Base(path)), nil
}
