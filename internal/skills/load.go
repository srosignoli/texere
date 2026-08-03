package skills

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The two shipped skills. They are embedded rather than written to disk on
// first run so that improvements reach existing installs, and so that deleting
// a user's copy restores the original — "reset to default" for free.
//
//go:embed builtin/*.md
var builtinFS embed.FS

const skillExt = ".md"

// Dir is where user-authored skills live: alongside config.json, because a
// learning method is personal rather than per-collection — how you want to be
// taught applies whether the collection is Latin or Kubernetes.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("skills: UserConfigDir: %w", err)
	}
	return filepath.Join(base, "texere", "skills"), nil
}

// List returns the builtins overlaid with any user files, sorted by name.
//
// A user file whose id matches a builtin REPLACES it and is marked Overridden;
// there is never one id twice. Sorting is by name regardless of origin so the
// order is predictable — origin is shown as a label, not as position.
//
// A missing directory is normal on first run and is not an error.
func List(dir string) ([]Skill, error) {
	byID := map[string]Skill{}

	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, fmt.Errorf("skills: read embedded: %w", err)
	}
	for _, e := range entries {
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("skills: read embedded %s: %w", e.Name(), err)
		}
		id := strings.TrimSuffix(e.Name(), skillExt)
		s := Parse(id, data)
		s.Builtin = true
		byID[id] = s
	}

	userEntries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("skills: read %s: %w", dir, err)
	}
	for _, e := range userEntries {
		// Case-insensitive: editors on Windows and macOS readily save ".MD" or
		// ".Md". A skill must never silently vanish (see parse.go), and a
		// case-sensitive suffix check is exactly that — the file is skipped
		// with no error and no log, so it just never appears.
		ext := filepath.Ext(e.Name())
		if e.IsDir() || !strings.EqualFold(ext, skillExt) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			// Skip files that cannot be read (bad permissions, broken symlink, etc).
			// One bad file must not prevent the built-ins and other user files from
			// being listed. The file simply does not appear in the list.
			continue
		}
		// Strip the file's ACTUAL extension (whatever case it was saved in),
		// not a lowercased guess: the id is what a notebook persists, and
		// lowercasing it would silently rename a user's skill out from under
		// any notebook already pointing at it.
		id := strings.TrimSuffix(e.Name(), ext)
		s := Parse(id, data)
		s.Path = path
		_, shadows := byID[id]
		s.Overridden = shadows
		byID[id] = s
	}

	out := make([]Skill, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a == b {
			return out[i].ID < out[j].ID
		}
		return a < b
	})
	return out, nil
}

// Duplicate copies a builtin's file into dir so the user can edit it. The copy
// keeps its frontmatter, so it remains a well-formed skill.
//
// It REFUSES when the target exists. The user file is the one thing here Texere
// did not author, and replacing an edited skill with the shipped text would
// destroy work with no undo.
func Duplicate(dir, id string) (string, error) {
	data, err := builtinFS.ReadFile("builtin/" + id + skillExt)
	if err != nil {
		return "", fmt.Errorf("skills: no builtin %q", id)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("skills: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, id+skillExt)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("skills: %s already exists", path)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("skills: write %s: %w", path, err)
	}
	return path, nil
}
