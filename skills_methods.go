package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"texere/internal/skills"
)

// revealEnv is the seam that makes opener selection testable: which platform we
// are on, which commands actually exist, and how to turn a POSIX path into a
// Windows one. Injected so a test can describe a machine exactly — "WSL, which
// has explorer.exe but no xdg-open" — without needing that machine.
type revealEnv struct {
	goos     string
	lookPath func(name string) (string, error)
	winPath  func(dir string) (string, error)
}

func osRevealEnv() revealEnv {
	return revealEnv{
		goos:     runtime.GOOS,
		lookPath: exec.LookPath,
		winPath:  wslWindowsPath,
	}
}

// wslWindowsPath converts a POSIX path to the Windows form explorer.exe needs
// (`\\wsl.localhost\<distro>\...`). Only meaningful under WSL, where wslpath
// exists; elsewhere it fails and the explorer.exe candidate is skipped.
func wslWindowsPath(dir string) (string, error) {
	out, err := exec.Command("wslpath", "-w", dir).Output()
	if err != nil {
		return "", err
	}
	converted := strings.TrimSpace(string(out))
	if converted == "" {
		return "", errors.New("wslpath produced an empty path")
	}
	return converted, nil
}

// revealCommand picks the first opener that actually exists on this machine.
//
// It probes rather than assuming, because assuming is what broke the shipped
// version: it hardcoded xdg-open for every non-Windows, non-macOS platform, and
// WSL2 has no xdg-open. The button created the skills directory and then failed
// with "executable file not found", which reads as though the directory were
// missing — so the error misdirected as well as failing.
//
// On Linux a real desktop wins over the WSL fallback. explorer.exe is treated as
// the WSL signal in its own right: if it is on PATH of a Linux box, we are under
// WSL and it is exactly the right thing to use, so no /proc/version sniffing is
// needed. It gets a converted path, since it cannot open a POSIX one, and the
// candidate is skipped entirely if that conversion fails rather than launching
// it with an argument it cannot use.
//
// When nothing is available the error NAMES the directory. An opener that cannot
// open should still tell the user where to go by hand; had the shipped version
// done that, this bug would have explained itself.
func revealCommand(env revealEnv, dir string) (name string, args []string, err error) {
	type candidate struct {
		name string
		args func() ([]string, error)
	}
	plain := func() ([]string, error) { return []string{dir}, nil }

	var candidates []candidate
	switch env.goos {
	case "windows":
		candidates = []candidate{{"explorer", plain}}
	case "darwin":
		candidates = []candidate{{"open", plain}}
	default:
		candidates = []candidate{
			{"xdg-open", plain},
			{"explorer.exe", func() ([]string, error) {
				win, err := env.winPath(dir)
				if err != nil {
					return nil, err
				}
				return []string{win}, nil
			}},
		}
	}

	tried := make([]string, 0, len(candidates))
	for _, c := range candidates {
		tried = append(tried, c.name)
		if _, err := env.lookPath(c.name); err != nil {
			continue
		}
		a, err := c.args()
		if err != nil {
			continue // e.g. wslpath failed; a raw POSIX path would not open
		}
		return c.name, a, nil
	}
	return "", nil, fmt.Errorf(
		"no file manager available (tried %s) — open %s yourself",
		strings.Join(tried, ", "), dir)
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

// SkillsDir reports where skill files live, so the settings tab can show the
// path outright. Authoring happens outside the app, and "Open skills folder"
// cannot be relied on — there is no file manager on a headless box, in a
// container, or under WSL without extra tooling. Showing the path makes the
// button a convenience rather than the only way in.
//
// It does not create the directory: this is a question, not an action.
func (a *App) SkillsDir() (string, error) {
	return skills.Dir()
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
	name, args, err := revealCommand(osRevealEnv(), dir)
	if err != nil {
		return err
	}
	return exec.Command(name, args...).Start()
}
