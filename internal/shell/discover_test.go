package shell

import (
	"errors"
	"testing"
)

// fakeEnv is the injected seam: a table of "installed" executables plus a GOOS
// and an env, so both the POSIX and the Windows branch are testable on any host.
type fakeEnv struct {
	goos    string
	env     map[string]string
	present map[string]string // arg to LookPath -> resolved path
	distros []string
}

func (f fakeEnv) Getenv(k string) string { return f.env[k] }
func (f fakeEnv) GOOS() string           { return f.goos }
func (f fakeEnv) WSLDistros() []string   { return f.distros }
func (f fakeEnv) LookPath(name string) (string, error) {
	if p, ok := f.present[name]; ok {
		return p, nil
	}
	return "", errors.New("not found")
}

func ids(shells []Shell) []string {
	out := make([]string, 0, len(shells))
	for _, s := range shells {
		out = append(out, s.ID)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDiscoverPosixPrefersSHELL(t *testing.T) {
	env := fakeEnv{
		goos: "linux",
		env:  map[string]string{"SHELL": "/usr/bin/fish"},
		present: map[string]string{
			"/usr/bin/fish": "/usr/bin/fish",
			"/bin/bash":     "/bin/bash",
			"/bin/sh":       "/bin/sh",
		},
	}
	got := ids(Discover(env))
	if want := []string{"fish", "bash", "sh"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDiscoverPosixWithoutSHELL(t *testing.T) {
	env := fakeEnv{
		goos:    "linux",
		env:     map[string]string{},
		present: map[string]string{"/bin/bash": "/bin/bash", "/bin/sh": "/bin/sh"},
	}
	got := ids(Discover(env))
	if want := []string{"bash", "sh"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// $SHELL is usually /bin/bash, which is also a fixed candidate. Listing bash
// twice in the dropdown would look like a bug.
func TestDiscoverPosixDedupesByResolvedPath(t *testing.T) {
	env := fakeEnv{
		goos:    "linux",
		env:     map[string]string{"SHELL": "/bin/bash"},
		present: map[string]string{"/bin/bash": "/bin/bash", "/bin/sh": "/bin/sh"},
	}
	got := ids(Discover(env))
	if want := []string{"bash", "sh"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDiscoverPosixAllKindPosix(t *testing.T) {
	env := fakeEnv{goos: "linux", present: map[string]string{"/bin/bash": "/bin/bash"}}
	for _, s := range Discover(env) {
		if s.Kind != KindPosix {
			t.Fatalf("%s: got kind %q, want %q", s.ID, s.Kind, KindPosix)
		}
	}
}

func TestDiscoverWindowsWithWSL(t *testing.T) {
	env := fakeEnv{
		goos: "windows",
		present: map[string]string{
			"pwsh.exe":       `C:\pwsh.exe`,
			"powershell.exe": `C:\powershell.exe`,
			"wsl.exe":        `C:\wsl.exe`,
			"cmd.exe":        `C:\cmd.exe`,
		},
		distros: []string{"Ubuntu", "Debian"},
	}
	got := ids(Discover(env))
	want := []string{"pwsh", "powershell", "wsl:Ubuntu", "wsl:Debian", "cmd"}
	if !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Each distro is its own entry, carrying the distro name for `wsl -d`.
func TestDiscoverWindowsDistroCarriesNameAndKind(t *testing.T) {
	env := fakeEnv{
		goos:    "windows",
		present: map[string]string{"wsl.exe": `C:\wsl.exe`},
		distros: []string{"Ubuntu"},
	}
	all := Discover(env)
	if len(all) != 1 {
		t.Fatalf("got %d shells, want 1", len(all))
	}
	s := all[0]
	if s.Kind != KindWSL || s.Distro != "Ubuntu" || s.Label != "Ubuntu (WSL)" || s.Path != `C:\wsl.exe` {
		t.Fatalf("got %+v", s)
	}
}

// WSL present but with no distro installed must contribute nothing — `wsl.exe`
// alone cannot run a command.
func TestDiscoverWindowsWSLWithNoDistros(t *testing.T) {
	env := fakeEnv{
		goos:    "windows",
		present: map[string]string{"wsl.exe": `C:\wsl.exe`, "cmd.exe": `C:\cmd.exe`},
		distros: nil,
	}
	got := ids(Discover(env))
	if want := []string{"cmd"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// The cell must be able to say "no shell found" rather than show an empty picker.
func TestDiscoverFindsNothing(t *testing.T) {
	if got := Discover(fakeEnv{goos: "linux"}); len(got) != 0 {
		t.Fatalf("got %v, want empty", ids(got))
	}
	if got := Discover(fakeEnv{goos: "windows"}); len(got) != 0 {
		t.Fatalf("got %v, want empty", ids(got))
	}
}
