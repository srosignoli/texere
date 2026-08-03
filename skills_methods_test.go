package main

import (
	"errors"
	"strings"
	"testing"
)

// fakeReveal builds a revealEnv whose LookPath succeeds only for the named
// commands, so a test can describe a machine precisely: "linux with a desktop",
// "WSL, which has explorer.exe but no xdg-open", "a bare container with
// neither".
func fakeReveal(goos string, present ...string) revealEnv {
	has := map[string]bool{}
	for _, p := range present {
		has[p] = true
	}
	return revealEnv{
		goos: goos,
		lookPath: func(name string) (string, error) {
			if has[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		winPath: func(dir string) (string, error) {
			return `\\wsl.localhost\Ubuntu` + strings.ReplaceAll(dir, "/", `\`), nil
		},
	}
}

func TestRevealCommandPrefersTheNativeOpener(t *testing.T) {
	const dir = "/some/dir"
	name, args, err := revealCommand(fakeReveal("linux", "xdg-open", "explorer.exe"), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "xdg-open" {
		t.Fatalf("name = %q, want xdg-open — a real desktop must win over the WSL fallback", name)
	}
	if len(args) != 1 || args[0] != dir {
		t.Fatalf("args = %v, want [%s]", args, dir)
	}
}

// WSL2 has no xdg-open. It does have explorer.exe, which cannot open a POSIX
// path and so needs a converted one. This is the case that made the shipped
// button fail.
func TestRevealCommandFallsBackToExplorerOnWSL(t *testing.T) {
	const dir = "/home/u/.config/texere/skills"
	name, args, err := revealCommand(fakeReveal("linux", "explorer.exe"), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "explorer.exe" {
		t.Fatalf("name = %q, want explorer.exe", name)
	}
	if len(args) != 1 || !strings.HasPrefix(args[0], `\\wsl.localhost\`) {
		t.Fatalf("args = %v, want a converted Windows path", args)
	}
}

// An opener that cannot open must still say WHERE the folder is, or the user has
// no way to reach it. The shipped failure said only "executable file not found",
// which read as though the directory were missing.
func TestRevealCommandErrorNamesTheDirectory(t *testing.T) {
	const dir = "/home/u/.config/texere/skills"
	_, _, err := revealCommand(fakeReveal("linux"), dir)
	if err == nil {
		t.Fatal("want an error when no opener is available")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("error %q does not name the directory %q", err, dir)
	}
}

// If the path cannot be converted, explorer.exe would be handed a POSIX path it
// cannot open. Skipping the candidate yields the actionable error instead.
func TestRevealCommandSkipsExplorerWhenPathConversionFails(t *testing.T) {
	env := fakeReveal("linux", "explorer.exe")
	env.winPath = func(string) (string, error) { return "", errors.New("wslpath: no") }
	_, _, err := revealCommand(env, "/some/dir")
	if err == nil {
		t.Fatal("want an error rather than explorer.exe with an unusable path")
	}
	if !strings.Contains(err.Error(), "/some/dir") {
		t.Fatalf("error %q does not name the directory", err)
	}
}

func TestRevealCommandWindowsAndDarwin(t *testing.T) {
	const dir = `C:\Users\u\skills`
	name, args, err := revealCommand(fakeReveal("windows", "explorer"), dir)
	if err != nil || name != "explorer" || len(args) != 1 || args[0] != dir {
		t.Fatalf("windows: name=%q args=%v err=%v", name, args, err)
	}
	name, args, err = revealCommand(fakeReveal("darwin", "open"), "/Users/u/skills")
	if err != nil || name != "open" || len(args) != 1 || args[0] != "/Users/u/skills" {
		t.Fatalf("darwin: name=%q args=%v err=%v", name, args, err)
	}
}

// Even where the platform opener is normally guaranteed, a missing one must not
// strand the user without a path to navigate to by hand.
func TestRevealCommandErrorNamesDirectoryOnEveryPlatform(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "freebsd"} {
		t.Run(goos, func(t *testing.T) {
			_, _, err := revealCommand(fakeReveal(goos), "/the/dir")
			if err == nil || !strings.Contains(err.Error(), "/the/dir") {
				t.Fatalf("err = %v, want one naming /the/dir", err)
			}
		})
	}
}
