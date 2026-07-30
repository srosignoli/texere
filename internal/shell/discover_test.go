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
	present map[string]string // arg to LookPath -> path LookPath returns
	links   map[string]string // path -> real file it is a symlink to
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

// EvalSymlinks models the /bin -> usr/bin layout that string-keyed dedup misses.
// An unlisted path is its own real file, as filepath.EvalSymlinks returns a
// non-symlink unchanged.
func (f fakeEnv) EvalSymlinks(p string) (string, error) {
	if r, ok := f.links[p]; ok {
		return r, nil
	}
	return p, nil
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
func TestDiscoverPosixDedupesIdenticalPaths(t *testing.T) {
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

// The case string-keyed dedup cannot catch. On Arch and Fedora /bin is a
// symlink to usr/bin, so `chsh -s /usr/bin/zsh` puts "/usr/bin/zsh" in $SHELL
// while "/bin/zsh" is still a fixed candidate: two different strings naming one
// binary. Both would be added, both with ID "zsh", and ShellCell.svelte's
// `{#each shells as s (s.id)}` throws on a duplicate key.
func TestDiscoverPosixDedupesByResolvedPath(t *testing.T) {
	env := fakeEnv{
		goos: "linux",
		env:  map[string]string{"SHELL": "/usr/bin/zsh"},
		present: map[string]string{
			"/usr/bin/zsh": "/usr/bin/zsh",
			"/bin/zsh":     "/bin/zsh",
			"/bin/sh":      "/bin/sh",
		},
		links: map[string]string{
			"/bin/zsh": "/usr/bin/zsh",  // /bin -> usr/bin
			"/bin/sh":  "/usr/bin/dash", // a genuinely different binary
		},
	}
	got := ids(Discover(env))
	if want := []string{"zsh", "sh"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Resolution is not enough on its own: /opt/homebrew/bin/bash and /bin/bash are
// two real, different binaries that both base-name to "bash". The ID is the
// dropdown's key, so the duplicate has to be dropped even though the paths
// differ.
func TestDiscoverPosixDedupesByID(t *testing.T) {
	env := fakeEnv{
		goos: "linux",
		env:  map[string]string{"SHELL": "/opt/homebrew/bin/bash"},
		present: map[string]string{
			"/opt/homebrew/bin/bash": "/opt/homebrew/bin/bash",
			"/bin/bash":              "/bin/bash",
			"/bin/sh":                "/bin/sh",
		},
	}
	got := ids(Discover(env))
	if want := []string{"bash", "sh"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// The one that survives must be the user's own choice, not the fallback.
	if p := Discover(env)[0].Path; p != "/opt/homebrew/bin/bash" {
		t.Fatalf("kept %q, want the $SHELL path", p)
	}
}

// A path that cannot be resolved must still be offered — a shell behind a
// permission-denied directory is better listed than silently dropped.
func TestDiscoverPosixKeepsShellsThatCannotBeResolved(t *testing.T) {
	env := fakeEnv{
		goos:    "linux",
		env:     map[string]string{"SHELL": "/usr/bin/fish"},
		present: map[string]string{"/usr/bin/fish": "/usr/bin/fish"},
	}
	got := ids(Discover(failingResolver{env}))
	if want := []string{"fish"}; !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// failingResolver is fakeEnv with EvalSymlinks always failing.
type failingResolver struct{ fakeEnv }

func (failingResolver) EvalSymlinks(string) (string, error) { return "", errors.New("nope") }

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
