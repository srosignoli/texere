package main

import (
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/skills"
)

// ListSkills returns the learning skills available to an LLM cell: the two
// embedded defaults overlaid with any Markdown files in the user's skills
// directory. Read on demand rather than cached, so editing a file in your own
// editor shows up the next time the picker opens — no watcher required.
func (a *App) ListSkills() ([]skills.Skill, error) {
	dir, err := skills.Dir()
	if err != nil {
		return nil, err
	}
	list, err := skills.List(dir)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return []skills.Skill{}, nil
	}
	return list, nil
}

// DuplicateSkill writes an editable copy of a builtin into the skills
// directory, where it then shadows the original. It fails rather than
// overwriting an existing file.
func (a *App) DuplicateSkill(id string) error {
	dir, err := skills.Dir()
	if err != nil {
		return err
	}
	_, err = skills.Duplicate(dir, id)
	return err
}

// RevealSkillsDir opens the skills directory in the OS file manager, creating
// it first so the user is never sent to a path that does not exist. Authoring
// happens in the user's own editor; this is the whole of the in-app support.
func (a *App) RevealSkillsDir() error {
	dir, err := skills.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(dir))
	return nil
}
