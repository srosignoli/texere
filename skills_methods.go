package main

import (
	"os"
	"os/exec"
	"runtime"

	"texere/internal/skills"
)

// revealCommand returns the OS-specific command that opens dir in the file
// manager, given the value of runtime.GOOS. Split out from RevealSkillsDir so
// the platform decision is table-testable; actually launching the command is
// not (there is no file manager in CI).
func revealCommand(goos, dir string) (name string, args []string) {
	switch goos {
	case "windows":
		return "explorer", []string{dir}
	case "darwin":
		return "open", []string{dir}
	default:
		// xdg-open covers Linux and the other Unix-likes Wails targets.
		return "xdg-open", []string{dir}
	}
}

// ListSkills returns the learning skills available to an LLM cell: the two
// embedded defaults overlaid with any Markdown files in the user's skills
// directory. Read on demand rather than cached, so editing a file in your own
// editor shows up the next time the picker opens — no watcher required.
//
// skills.List builds its result with make([]Skill, 0, ...) on every success
// path (internal/skills/load.go), so a nil slice can only occur alongside a
// non-nil err — there is no empty-but-nil case here to guard against before
// this crosses into JSON, unlike a bare `var s []Skill`.
func (a *App) ListSkills() ([]skills.Skill, error) {
	dir, err := skills.Dir()
	if err != nil {
		return nil, err
	}
	return skills.List(dir)
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
//
// Wails' runtime.BrowserOpenURL is not usable here: it routes through
// ValidateAndSanitizeURL, which rejects the file:// scheme on every platform
// backend. A plain OS command is used instead. Start(), not Run() — the file
// manager is a long-lived process, and on Windows explorer.exe exits non-zero
// even on success, which Start() never observes.
func (a *App) RevealSkillsDir() error {
	dir, err := skills.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name, args := revealCommand(runtime.GOOS, dir)
	return exec.Command(name, args...).Start()
}
