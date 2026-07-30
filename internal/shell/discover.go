package shell

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Env is the seam that makes discovery testable. Discovery is the one part of
// this package whose behaviour depends on the host OS, and this repo
// cross-compiles for Windows — without this interface the Windows branch could
// never be tested on a Linux dev machine, nor the POSIX branch on Windows.
type Env interface {
	Getenv(key string) string
	LookPath(file string) (string, error)
	// EvalSymlinks resolves a path to the real file it names, so two different
	// strings naming one binary can be recognised as one shell. Part of the
	// seam because symlink layouts differ per distro and cannot be faked by
	// LookPath alone.
	EvalSymlinks(path string) (string, error)
	GOOS() string
	WSLDistros() []string
}

type osEnv struct{}

// OSEnv returns the real environment.
func OSEnv() Env { return osEnv{} }

func (osEnv) Getenv(k string) string                { return os.Getenv(k) }
func (osEnv) LookPath(f string) (string, error)     { return exec.LookPath(f) }
func (osEnv) EvalSymlinks(p string) (string, error) { return filepath.EvalSymlinks(p) }
func (osEnv) GOOS() string                          { return runtime.GOOS }

// WSLDistros lists installed distros. `wsl -l -q` emits UTF-16 with CRs on many
// builds, so the BOM, every NUL and every CR are stripped before splitting.
// The BOM matters: without it the first distro's first character is corrupted
// and `wsl -d <name>` then fails to find it.
func (osEnv) WSLDistros() []string {
	out, err := exec.Command("wsl.exe", "-l", "-q").Output()
	if err != nil {
		return nil
	}
	out = bytes.TrimPrefix(out, []byte{0xFF, 0xFE}) // UTF-16LE BOM
	cleaned := strings.NewReplacer("\x00", "", "\r", "").Replace(string(out))
	var names []string
	for _, line := range strings.Split(cleaned, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// Discover returns the shells actually installed, in the order they should
// appear in the dropdown. An empty result is meaningful: the cell reports that
// no shell was found rather than showing an empty picker.
func Discover(env Env) []Shell {
	if env.GOOS() == "windows" {
		return discoverWindows(env)
	}
	return discoverPosix(env)
}

func discoverPosix(env Env) []Shell {
	// $SHELL first — it is what the user actually chose — then the usual
	// suspects as fallbacks.
	candidates := []string{env.Getenv("SHELL"), "/bin/bash", "/bin/zsh", "/bin/sh"}

	var out []Shell
	seenPath := map[string]bool{}
	seenID := map[string]bool{}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		path, err := env.LookPath(c)
		if err != nil {
			continue
		}
		// Dedupe on the RESOLVED path, not on the string LookPath handed back.
		// Where /bin is a symlink to usr/bin — Arch, Fedora — a user who ran
		// `chsh -s /usr/bin/zsh` yields "/usr/bin/zsh" from $SHELL and
		// "/bin/zsh" from the candidate list: two strings, one binary. If
		// resolution fails (a dangling link, a permission error) fall back to
		// the unresolved path rather than dropping the shell.
		resolved := path
		if r, err := env.EvalSymlinks(path); err == nil {
			resolved = r
		}
		// The ID is checked separately because two genuinely different binaries
		// can share a base name — Homebrew's /opt/homebrew/bin/bash alongside
		// /bin/bash. Either kind of duplicate breaks ShellCell.svelte's
		// `{#each shells as s (s.id)}`, which throws on a repeated key.
		name := filepath.Base(path)
		if seenPath[resolved] || seenID[name] {
			continue
		}
		seenPath[resolved] = true
		seenID[name] = true
		out = append(out, Shell{ID: name, Label: name, Path: path, Kind: KindPosix})
	}
	return out
}

func discoverWindows(env Env) []Shell {
	var out []Shell

	for _, c := range []struct{ file, id, label string }{
		{"pwsh.exe", "pwsh", "PowerShell"},
		{"powershell.exe", "powershell", "Windows PowerShell"},
	} {
		if path, err := env.LookPath(c.file); err == nil {
			out = append(out, Shell{ID: c.id, Label: c.label, Path: path, Kind: KindPowerShell})
		}
	}

	// wsl.exe on its own cannot run anything — a distro is required — so a WSL
	// install with no distro contributes no entries.
	if path, err := env.LookPath("wsl.exe"); err == nil {
		for _, d := range env.WSLDistros() {
			out = append(out, Shell{
				ID: "wsl:" + d, Label: d + " (WSL)", Path: path, Kind: KindWSL, Distro: d,
			})
		}
	}

	if path, err := env.LookPath("cmd.exe"); err == nil {
		out = append(out, Shell{ID: "cmd", Label: "cmd", Path: path, Kind: KindCmd})
	}
	return out
}
