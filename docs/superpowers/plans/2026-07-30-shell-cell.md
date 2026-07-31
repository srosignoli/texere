# Shell Cell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A notebook cell that runs a command in a chosen OS shell, streams the output back, and promotes a selected part of that output into N4L as a fact about the command that produced it.

**Architecture:** A new pure-Go `internal/shell` package (discovery, invocation, streaming, process-group cancellation, batching) exposed through `shell_methods.go` using the same start/emit/cancel shape as the existing LLM turn API. On the frontend, `shell-fact.js` builds the N4L (pure, unit-tested), `shell-run.js` wires the Wails events, and `ShellCell.svelte` is the UI. No PTY, no new dependencies.

**Tech Stack:** Go 1.25 (`os/exec`, `syscall`), Wails v2 events, Svelte 3.49, `node:test`.

## Global Constraints

- **No PTY, and stdin is closed.** With no PTY, anything that prompts would hang forever with no way to answer. Closing stdin makes `sudo` fail immediately with a readable error instead of hanging the cell; PowerShell gets `-NonInteractive` for the same reason. This is deliberate — do not add a PTY, and do not wire stdin.
- **Cancel MUST kill the process group, not just the child.** `exec.CommandContext` signals only the direct process, so `bash -c "sleep 100 | cat"` would leave an orphan. Every Stop that leaks a process is a defect.
- **Three exec-posture invariants:** never run on notebook load; never run when `readOnly`; run only on an explicit Run. No confirmation dialogs, no allowlists, no blocklists — the user's own machine and shell.
- **Scrollback is NEVER persisted.** `SaveNotebook(nbId, JSON.stringify(cells))` writes every cell as one JSON blob on a 600ms debounce, so output in `cell` would rewrite the whole notebook repeatedly at megabyte scale. Output lives in component-local state only.
- **A non-zero exit is information, not a failure.** Render it in the output pane; never raise an error toast. `grep` returning 1 is normal.
- **All new dependencies MUST be pure Go (no CGo).** This feature needs none at all — `os/exec` and `syscall` are stdlib.
- **Arrow codes `cmd`, `out`, `outof`, `runfor` are verified free.** Do not use `ran` — it sits confusingly beside the existing `runs`, which is the collision class recorded in commit `582ca30`.
- Follow the existing style of the files edited: Go `gofmt`, imports grouped stdlib/external/internal; JS 2-space indent, NO semicolons, single quotes.
- Run `make test`, `make test-js`, and `cd frontend && npx vite build` before each commit.

---

### Task 1: Shell discovery

**Files:**
- Create: `internal/shell/shell.go`
- Create: `internal/shell/discover.go`
- Create: `internal/shell/discover_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Kind string` with constants `KindPosix`, `KindPowerShell`, `KindCmd`, `KindWSL` (values `"posix"`, `"powershell"`, `"cmd"`, `"wsl"`).
  - `type Shell struct { ID, Label, Path string; Kind Kind; Distro string }` with JSON tags `id`, `label`, `path`, `kind`, `distro`.
  - `type Env interface { Getenv(string) string; LookPath(string) (string, error); GOOS() string; WSLDistros() []string }`
  - `func OSEnv() Env` — the real implementation.
  - `func Discover(env Env) []Shell`

**Why the `Env` seam.** Discovery is the one part of this feature that behaves
differently per platform, and this repo cross-compiles for Windows
(`make build-windows`). Without an injected seam, the Windows branch is
untestable on the Linux dev machine and the Linux branch is untestable on
Windows — so neither ever gets tested. `Env` makes both branches testable
everywhere.

- [ ] **Step 1: Write the failing test**

Create `internal/shell/discover_test.go`:

```go
package shell

import (
	"errors"
	"testing"
)

// fakeEnv is the injected seam: a table of "installed" executables plus a GOOS
// and an env, so both the POSIX and the Windows branch are testable on any host.
type fakeEnv struct {
	goos     string
	env      map[string]string
	present  map[string]string // arg to LookPath -> resolved path
	distros  []string
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/shell/`
Expected: FAIL — the package does not exist yet (`no Go files` or undefined `Discover`).

- [ ] **Step 3: Write `internal/shell/shell.go`**

```go
// Package shell runs a single command in an OS shell and streams its output.
//
// It is deliberately NOT a terminal: there is no PTY and stdin is closed, so
// anything that prompts (sudo without NOPASSWD, ssh, vim, top) fails fast with
// a readable error rather than hanging forever with no way to answer. That is
// the accepted cost of not carrying a PTY dependency; see the shell-cell design
// doc for the reasoning.
package shell

// Kind selects how a command is handed to a shell. Every shell in one Kind
// takes the same argument shape (see ArgsFor).
type Kind string

const (
	KindPosix      Kind = "posix"
	KindPowerShell Kind = "powershell"
	KindCmd        Kind = "cmd"
	KindWSL        Kind = "wsl"
)

// Shell is one entry in the cell's dropdown.
//
// ID is the stable key persisted on the cell — it must not change between
// releases or a saved notebook loses its shell selection. Label is display-only.
// Distro is set for KindWSL alone, and is what `wsl -d` needs.
type Shell struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Kind   Kind   `json:"kind"`
	Distro string `json:"distro,omitempty"`
}
```

- [ ] **Step 4: Write `internal/shell/discover.go`**

```go
package shell

import (
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
	GOOS() string
	WSLDistros() []string
}

type osEnv struct{}

// OSEnv returns the real environment.
func OSEnv() Env { return osEnv{} }

func (osEnv) Getenv(k string) string                 { return os.Getenv(k) }
func (osEnv) LookPath(f string) (string, error)       { return exec.LookPath(f) }
func (osEnv) GOOS() string                            { return runtime.GOOS }

// WSLDistros lists installed distros. `wsl -l -q` emits UTF-16 with CRs on many
// builds, so every NUL and CR is stripped before splitting.
func (osEnv) WSLDistros() []string {
	out, err := exec.Command("wsl.exe", "-l", "-q").Output()
	if err != nil {
		return nil
	}
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
	seen := map[string]bool{}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		path, err := env.LookPath(c)
		if err != nil || seen[path] {
			continue
		}
		seen[path] = true
		name := filepath.Base(path)
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
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/shell/ -v`
Expected: PASS — all eight tests.

- [ ] **Step 6: Commit**

```bash
git add internal/shell/shell.go internal/shell/discover.go internal/shell/discover_test.go
git commit -m "feat(shell): discover the shells actually installed"
```

---

### Task 2: Invocation args and Run

**Files:**
- Create: `internal/shell/run.go`
- Create: `internal/shell/run_test.go`

**Interfaces:**
- Consumes: `Shell`, `Kind` and its constants from Task 1.
- Produces:
  - `func ArgsFor(sh Shell, command string) []string`
  - `type Chunk struct { Stream, Text string }` — JSON tags `stream`, `text`; `Stream` is `"stdout"` or `"stderr"`.
  - `type Result struct { ExitCode int; DurationMs int64 }` — JSON tags `exitCode`, `durationMs`.
  - `func Run(ctx context.Context, sh Shell, cwd, command string, sink func(Chunk)) (Result, error)`

Task 3 adds process-group cancellation to `Run` via a platform hook; do not
attempt that here.

**On ordering, honestly:** stdout and stderr are read by two goroutines, so
`sink` calls are serialised by a mutex but the *interleaving* is best-effort — it
cannot be guaranteed to match the child's write order. Tests must therefore
assert per-stream content and ordering *within* a stream, never a specific
cross-stream interleaving. A test that asserts one is a flaky test.

- [ ] **Step 1: Write the failing test**

Create `internal/shell/run_test.go`:

```go
package shell

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestArgsFor(t *testing.T) {
	cases := []struct {
		name string
		sh   Shell
		want []string
	}{
		{"posix", Shell{Kind: KindPosix}, []string{"-c", "echo hi"}},
		{"powershell", Shell{Kind: KindPowerShell}, []string{"-NoProfile", "-NonInteractive", "-Command", "echo hi"}},
		{"cmd", Shell{Kind: KindCmd}, []string{"/C", "echo hi"}},
		{"wsl", Shell{Kind: KindWSL, Distro: "Ubuntu"}, []string{"-d", "Ubuntu", "--", "bash", "-lc", "echo hi"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ArgsFor(c.sh, "echo hi")
			if strings.Join(got, "\x00") != strings.Join(c.want, "\x00") {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

// -NonInteractive is what makes a PowerShell prompt fail instead of hanging.
func TestArgsForPowerShellIsNonInteractive(t *testing.T) {
	args := ArgsFor(Shell{Kind: KindPowerShell}, "whatever")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-NonInteractive") || !strings.Contains(joined, "-NoProfile") {
		t.Fatalf("got %q", joined)
	}
}

// collector serialises what Run streams, per stream.
type collector struct {
	mu     sync.Mutex
	stdout strings.Builder
	stderr strings.Builder
}

func (c *collector) sink(ch Chunk) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch.Stream == "stderr" {
		c.stderr.WriteString(ch.Text)
		return
	}
	c.stdout.WriteString(ch.Text)
}

func shTest(t *testing.T) Shell {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test")
	}
	return Shell{ID: "sh", Label: "sh", Path: "/bin/sh", Kind: KindPosix}
}

func TestRunCapturesStdoutAndExitZero(t *testing.T) {
	c := &collector{}
	res, err := Run(context.Background(), shTest(t), "", "echo hello", c.sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("got exit %d, want 0", res.ExitCode)
	}
	if got := strings.TrimSpace(c.stdout.String()); got != "hello" {
		t.Fatalf("got stdout %q, want %q", got, "hello")
	}
}

// A non-zero exit is information, not an error — Run must report the code and
// return nil, or the caller cannot tell a failed command from a broken spawn.
func TestRunNonZeroExitIsNotAnError(t *testing.T) {
	c := &collector{}
	res, err := Run(context.Background(), shTest(t), "", "exit 3", c.sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 3 {
		t.Fatalf("got exit %d, want 3", res.ExitCode)
	}
}

func TestRunTagsStderrSeparately(t *testing.T) {
	c := &collector{}
	if _, err := Run(context.Background(), shTest(t), "", "echo out; echo err 1>&2", c.sink); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(c.stdout.String()); got != "out" {
		t.Fatalf("got stdout %q, want %q", got, "out")
	}
	if got := strings.TrimSpace(c.stderr.String()); got != "err" {
		t.Fatalf("got stderr %q, want %q", got, "err")
	}
}

// Ordering is only guaranteed within a stream.
func TestRunPreservesOrderWithinAStream(t *testing.T) {
	c := &collector{}
	if _, err := Run(context.Background(), shTest(t), "", "for i in 1 2 3 4 5; do echo $i; done", c.sink); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Fields(c.stdout.String()); strings.Join(got, "") != "12345" {
		t.Fatalf("got %v, want 1..5 in order", got)
	}
}

func TestRunHonoursCwd(t *testing.T) {
	dir := t.TempDir()
	c := &collector{}
	if _, err := Run(context.Background(), shTest(t), dir, "pwd", c.sink); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// macOS resolves TempDir through /private, so compare suffixes.
	if got := strings.TrimSpace(c.stdout.String()); !strings.HasSuffix(got, strings.TrimPrefix(dir, "/private")) {
		t.Fatalf("got pwd %q, want it to end with %q", got, dir)
	}
}

// Failing before spawn gives a clear message instead of a shell-specific one.
func TestRunRejectsMissingCwdBeforeSpawn(t *testing.T) {
	c := &collector{}
	_, err := Run(context.Background(), shTest(t), "/definitely/not/here", "echo hi", c.sink)
	if err == nil {
		t.Fatal("want an error for a missing cwd")
	}
	if c.stdout.Len() != 0 || c.stderr.Len() != 0 {
		t.Fatal("nothing should have been streamed")
	}
}

func TestRunRejectsCwdThatIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &collector{}
	if _, err := Run(context.Background(), shTest(t), file, "echo hi", c.sink); err == nil {
		t.Fatal("want an error when cwd is a file")
	}
}

// stdin is closed on purpose: with no PTY a prompt would hang forever. Reading
// stdin must therefore reach EOF immediately, not block.
func TestRunClosesStdin(t *testing.T) {
	c := &collector{}
	res, err := Run(context.Background(), shTest(t), "", "cat; echo done", c.sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("got exit %d, want 0", res.ExitCode)
	}
	if got := strings.TrimSpace(c.stdout.String()); got != "done" {
		t.Fatalf("got %q, want %q", got, "done")
	}
}

func TestRunReportsSpawnFailure(t *testing.T) {
	c := &collector{}
	bad := Shell{ID: "nope", Path: "/definitely/not/a/shell", Kind: KindPosix}
	if _, err := Run(context.Background(), bad, "", "echo hi", c.sink); err == nil {
		t.Fatal("want an error when the shell binary does not exist")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/shell/`
Expected: FAIL — `ArgsFor`, `Run`, `Chunk` and `Result` are all undefined.

- [ ] **Step 3: Write `internal/shell/run.go`**

```go
package shell

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Chunk is one piece of output, tagged with the stream it came from so the UI
// can tint stderr without reordering anything.
type Chunk struct {
	Stream string `json:"stream"` // "stdout" | "stderr"
	Text   string `json:"text"`
}

// Result reports how a finished run ended. A non-zero ExitCode is information,
// not an error — Run returns a nil error for it.
type Result struct {
	ExitCode   int   `json:"exitCode"`
	DurationMs int64 `json:"durationMs"`
}

// ArgsFor builds the argv that hands `command` to `sh` as a single string.
//
// PowerShell gets -NoProfile so a user's profile cannot change behaviour or
// print banners, and -NonInteractive so a prompt errors instead of hanging.
// WSL needs -d to pick the distro, then a login shell inside it so PATH is set
// the way the user expects.
func ArgsFor(sh Shell, command string) []string {
	switch sh.Kind {
	case KindPowerShell:
		return []string{"-NoProfile", "-NonInteractive", "-Command", command}
	case KindCmd:
		return []string{"/C", command}
	case KindWSL:
		return []string{"-d", sh.Distro, "--", "bash", "-lc", command}
	default:
		return []string{"-c", command}
	}
}

// Run executes command in sh and streams tagged output to sink until the
// process exits or ctx is cancelled.
//
// sink is called from two reader goroutines but is serialised by a mutex, so
// implementations need no locking of their own. Interleaving BETWEEN stdout and
// stderr is best-effort and must not be relied upon; order within one stream is
// preserved.
//
// A non-zero exit yields (Result{ExitCode: n}, nil). A non-nil error means the
// run never properly started or the pipes broke — a different situation for the
// caller, which reports it as a cell error rather than as command output.
func Run(ctx context.Context, sh Shell, cwd, command string, sink func(Chunk)) (Result, error) {
	if cwd != "" {
		info, err := os.Stat(cwd)
		if err != nil {
			return Result{}, fmt.Errorf("working directory %q: %w", cwd, err)
		}
		if !info.IsDir() {
			return Result{}, fmt.Errorf("working directory %q is not a directory", cwd)
		}
	}
	if sh.Path == "" {
		return Result{}, errors.New("no shell selected")
	}

	cmd := exec.CommandContext(ctx, sh.Path, ArgsFor(sh, command)...)
	cmd.Dir = cwd
	// Closed on purpose — see the package comment. exec's zero Stdin is already
	// /dev/null; stating it stops anyone "fixing" it into os.Stdin, which in a
	// GUI app is not a terminal and would block forever.
	cmd.Stdin = nil
	setProcAttr(cmd)
	cmd.Cancel = func() error { return killGroup(cmd) }
	// Insurance against a hang: the output pumps below read until EOF, and EOF
	// only arrives when every writer has closed the pipe. If a grandchild
	// survives killGroup and keeps its end open, Run would block forever.
	// WaitDelay forces the pipes closed shortly after cancellation.
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, err
	}

	started := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	pump := func(r io.Reader, stream string) {
		defer wg.Done()
		br := bufio.NewReader(r)
		buf := make([]byte, 8192)
		for {
			n, err := br.Read(buf)
			if n > 0 {
				mu.Lock()
				sink(Chunk{Stream: stream, Text: string(buf[:n])})
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go pump(stdout, "stdout")
	go pump(stderr, "stderr")
	wg.Wait()

	waitErr := cmd.Wait()
	res := Result{DurationMs: time.Since(started).Milliseconds()}

	var exitErr *exec.ExitError
	switch {
	case waitErr == nil:
		res.ExitCode = 0
	case errors.As(waitErr, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return res, waitErr
	}
	return res, nil
}
```

- [ ] **Step 4: Add temporary platform stubs so the package compiles**

Task 3 replaces both of these with real implementations. Create
`internal/shell/procattr.go`:

```go
package shell

import "os/exec"

// Replaced in Task 3 by per-platform implementations that put the child in its
// own process group and kill the whole group. Until then, cancellation falls
// back to signalling only the direct child, which leaks pipeline members.
func setProcAttr(c *exec.Cmd) {}

func killGroup(c *exec.Cmd) error {
	if c.Process == nil {
		return nil
	}
	return c.Process.Kill()
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/shell/ -v`
Expected: PASS — Task 1's eight tests plus Task 2's eleven.

- [ ] **Step 6: Commit**

```bash
git add internal/shell/run.go internal/shell/run_test.go internal/shell/procattr.go
git commit -m "feat(shell): run a command and stream tagged output"
```

---

### Task 3: Cancel kills the whole process group

**Files:**
- Delete: `internal/shell/procattr.go` (the Task 2 stub)
- Create: `internal/shell/procattr_unix.go`
- Create: `internal/shell/procattr_windows.go`
- Create: `internal/shell/cancel_test.go`

**Interfaces:**
- Consumes: `Run`, `Shell`, `Chunk` from Task 2.
- Produces: `setProcAttr(*exec.Cmd)` and `killGroup(*exec.Cmd) error` as
  build-tagged implementations. No exported API changes.

**Why this is its own task.** `exec.CommandContext` signals only the direct
child. `sh -c "sleep 30 | cat"` leaves `sleep` and `cat` alive after cancel, so
every Stop press leaks two processes that keep writing to a closed pipe. In a
tool aimed at devops this is not a rough edge, it is a defect — so it gets its
own test and its own review gate.

- [ ] **Step 1: Write the failing test**

Create `internal/shell/cancel_test.go`:

```go
package shell

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// pgrepCount counts live processes whose command line contains marker.
func pgrepCount(t *testing.T, marker string) int {
	t.Helper()
	out, err := exec.Command("ps", "-eo", "args").Output()
	if err != nil {
		t.Fatalf("ps failed: %v", err)
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, marker) && !strings.Contains(line, "ps -eo") {
			n++
		}
	}
	return n
}

// The whole point of this task: cancelling must not leave the pipeline behind.
func TestCancelKillsTheWholeProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses ps; the Windows path is verified manually")
	}
	sh := Shell{ID: "sh", Path: "/bin/sh", Kind: KindPosix}
	marker := "shellcell-group-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Two processes: a sleep and a cat downstream of it.
		_, _ = Run(ctx, sh, "", "sleep 30 | cat # "+marker, func(Chunk) {})
	}()

	// Give the shell time to fork the pipeline.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && pgrepCount(t, marker) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if pgrepCount(t, marker) == 0 {
		cancel()
		t.Fatal("pipeline never started")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	// Reaping is not instantaneous; poll briefly before declaring a leak.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pgrepCount(t, marker) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%d process(es) matching %q survived cancel", pgrepCount(t, marker), marker)
}

func TestCancelledRunReturnsPromptly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test")
	}
	sh := Shell{ID: "sh", Path: "/bin/sh", Kind: KindPosix}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()

	start := time.Now()
	if _, err := Run(ctx, sh, "", "sleep 30", func(Chunk) {}); err == nil {
		t.Log("cancel surfaced as a non-zero exit rather than an error; acceptable")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Run took %v to return after cancel", elapsed)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/shell/ -run TestCancelKillsTheWholeProcessGroup -v`
Expected: FAIL — "1 process(es) ... survived cancel", because the stub only kills
the direct child.

- [ ] **Step 3: Delete the stub and write the Unix implementation**

```bash
rm internal/shell/procattr.go
```

Create `internal/shell/procattr_unix.go`:

```go
//go:build !windows

package shell

import (
	"os/exec"
	"syscall"
)

// setProcAttr puts the child in a NEW process group whose leader is the child
// itself. That group id is what makes it possible to signal the shell and
// everything it forked in one call.
func setProcAttr(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup signals the whole group. Kill(-pid) means "every process in the
// group led by pid" — which is the difference between stopping `sleep 30 | cat`
// and leaving both halves running with a dead parent.
//
// SIGKILL rather than SIGTERM: this is a Stop button on a command the user has
// decided they do not want, and a shell that traps SIGTERM would otherwise
// ignore it.
func killGroup(c *exec.Cmd) error {
	if c.Process == nil {
		return nil
	}
	if err := syscall.Kill(-c.Process.Pid, syscall.SIGKILL); err != nil {
		// The group may already be gone, or Setpgid may not have taken effect;
		// fall back to the direct child rather than leaving it running.
		return c.Process.Kill()
	}
	return nil
}
```

- [ ] **Step 4: Write the Windows implementation**

Create `internal/shell/procattr_windows.go`:

```go
//go:build windows

package shell

import (
	"os/exec"
	"strconv"
)

// Windows has no process groups in the POSIX sense. CREATE_NEW_PROCESS_GROUP
// only affects Ctrl+C/Ctrl+Break delivery, which a GUI app cannot send, so it
// buys nothing here and is deliberately not set.
func setProcAttr(c *exec.Cmd) {}

// killGroup uses taskkill /T, which walks the child process tree — the closest
// equivalent to signalling a POSIX process group, and what stops a WSL or
// PowerShell child from surviving Stop. /F because a graceful request has no
// meaning for a command the user has abandoned.
func killGroup(c *exec.Cmd) error {
	if c.Process == nil {
		return nil
	}
	pid := strconv.Itoa(c.Process.Pid)
	if err := exec.Command("taskkill.exe", "/T", "/F", "/PID", pid).Run(); err != nil {
		return c.Process.Kill()
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/shell/ -v`
Expected: PASS — including `TestCancelKillsTheWholeProcessGroup`.

Then confirm the Windows file at least compiles, since CI and `make build-windows`
will build it:

Run: `GOOS=windows GOARCH=amd64 go build ./internal/shell/`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/shell/procattr_unix.go internal/shell/procattr_windows.go internal/shell/cancel_test.go
git commit -m "fix(shell): kill the process group on cancel, not just the child"
```

---

### Task 4: Batching output

**Files:**
- Create: `internal/shell/batch.go`
- Create: `internal/shell/batch_test.go`

**Interfaces:**
- Consumes: `Chunk` from Task 2.
- Produces: `type Batcher struct{ ... }`, `func NewBatcher(max int) *Batcher`,
  `(*Batcher).Add(Chunk)`, `(*Batcher).Take() []Chunk`.

**Why.** `find /` produces hundreds of thousands of lines. One Wails event per
read would flood the bridge regardless of memory. `Batcher` accumulates chunks
so the caller can emit them on a ticker. It holds no timer of its own precisely
so its tests are deterministic — the ticker lives in `shell_methods.go`.

Chunks for the same stream are concatenated as they are added, so a batch is a
handful of chunks rather than thousands of one-line ones.

- [ ] **Step 1: Write the failing test**

Create `internal/shell/batch_test.go`:

```go
package shell

import (
	"strings"
	"sync"
	"testing"
)

func TestBatcherTakeEmptiesTheBuffer(t *testing.T) {
	b := NewBatcher(1024)
	b.Add(Chunk{Stream: "stdout", Text: "a"})
	if got := b.Take(); len(got) != 1 || got[0].Text != "a" {
		t.Fatalf("got %+v", got)
	}
	if got := b.Take(); len(got) != 0 {
		t.Fatalf("second Take should be empty, got %+v", got)
	}
}

// The point of batching: many adds collapse into few chunks.
func TestBatcherCoalescesSameStream(t *testing.T) {
	b := NewBatcher(1024)
	for i := 0; i < 500; i++ {
		b.Add(Chunk{Stream: "stdout", Text: "x"})
	}
	got := b.Take()
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1", len(got))
	}
	if len(got[0].Text) != 500 {
		t.Fatalf("got %d bytes, want 500", len(got[0].Text))
	}
}

// Coalescing must never merge across streams, or stderr tinting breaks.
func TestBatcherKeepsStreamsSeparateAndOrdered(t *testing.T) {
	b := NewBatcher(1024)
	b.Add(Chunk{Stream: "stdout", Text: "a"})
	b.Add(Chunk{Stream: "stderr", Text: "b"})
	b.Add(Chunk{Stream: "stdout", Text: "c"})
	got := b.Take()
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3", len(got))
	}
	if got[0].Stream != "stdout" || got[1].Stream != "stderr" || got[2].Stream != "stdout" {
		t.Fatalf("streams out of order: %+v", got)
	}
}

// A single chunk must stay bounded so one Take cannot hand over 200MB.
func TestBatcherCapsChunkSize(t *testing.T) {
	b := NewBatcher(8)
	b.Add(Chunk{Stream: "stdout", Text: strings.Repeat("y", 40)})
	total := 0
	for _, c := range b.Take() {
		if len(c.Text) > 8 {
			t.Fatalf("chunk of %d bytes exceeds the 8-byte cap", len(c.Text))
		}
		total += len(c.Text)
	}
	if total != 40 {
		t.Fatalf("got %d bytes total, want 40 — capping must split, not drop", total)
	}
}

func TestBatcherIsSafeForConcurrentAdds(t *testing.T) {
	b := NewBatcher(1024)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.Add(Chunk{Stream: "stdout", Text: "z"})
			}
		}()
	}
	wg.Wait()
	total := 0
	for _, c := range b.Take() {
		total += len(c.Text)
	}
	if total != 800 {
		t.Fatalf("got %d bytes, want 800", total)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/shell/ -run TestBatcher`
Expected: FAIL — `NewBatcher` undefined.

- [ ] **Step 3: Write `internal/shell/batch.go`**

```go
package shell

import "sync"

// Batcher accumulates output chunks so a caller can emit them periodically
// instead of once per read. `find /` produces hundreds of thousands of lines,
// and one Wails event each would flood the bridge on its own.
//
// It deliberately holds NO timer: the ticker belongs to the caller, which keeps
// these tests deterministic rather than time-dependent.
type Batcher struct {
	mu    sync.Mutex
	buf   []Chunk
	maxCh int
}

// NewBatcher returns a Batcher whose individual chunks never exceed max bytes,
// so one Take cannot hand over a single enormous string.
func NewBatcher(max int) *Batcher {
	if max <= 0 {
		max = 64 << 10
	}
	return &Batcher{maxCh: max}
}

// Add appends text, concatenating onto the tail when it is the same stream and
// still under the cap. Chunks are never merged across streams — the UI tints
// stderr, so the boundary carries meaning.
func (b *Batcher) Add(c Chunk) {
	if c.Text == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	text := c.Text
	if n := len(b.buf); n > 0 {
		tail := &b.buf[n-1]
		if tail.Stream == c.Stream && len(tail.Text) < b.maxCh {
			room := b.maxCh - len(tail.Text)
			if room > len(text) {
				room = len(text)
			}
			tail.Text += text[:room]
			text = text[room:]
		}
	}
	// Anything that did not fit becomes further chunks, so capping splits
	// rather than drops.
	for len(text) > 0 {
		n := len(text)
		if n > b.maxCh {
			n = b.maxCh
		}
		b.buf = append(b.buf, Chunk{Stream: c.Stream, Text: text[:n]})
		text = text[n:]
	}
}

// Take returns everything buffered and empties the buffer.
func (b *Batcher) Take() []Chunk {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.buf) == 0 {
		return nil
	}
	out := b.buf
	b.buf = nil
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/shell/ -v`
Expected: PASS — all tests from Tasks 1-4.

- [ ] **Step 5: Commit**

```bash
git add internal/shell/batch.go internal/shell/batch_test.go
git commit -m "feat(shell): batch output so a large run cannot flood the bridge"
```

---

### Task 5: Wails bindings

**Files:**
- Modify: `app.go` (two fields in the `App` struct beside `llmStreams`/`llmMu`; one initialiser beside `a.llmStreams = ...`)
- Create: `shell_methods.go`

**Interfaces:**
- Consumes: everything from `internal/shell` — `Discover`, `OSEnv`, `Shell`,
  `Run`, `Chunk`, `Result`, `NewBatcher`, `ArgsFor`.
- Produces, for the frontend:
  - `ListShells() []shell.Shell`
  - `StartShellRun(req StartShellRunReq) (string, error)` where
    `StartShellRunReq{ShellID, Cwd, Command string}` with JSON tags
    `shellId`, `cwd`, `command`
  - `CancelShellRun(runID string)`
  - Events: `shell:out` `{runId, chunks: [{stream, text}]}`,
    `shell:exit` `{runId, exitCode, durationMs}`,
    `shell:error` `{runId, message}`

This mirrors `StartLLMTurn` / `CancelLLMTurn` in `llm_methods.go:115-159`
deliberately — same registry-of-cancel-funcs shape, same event naming, same
`newStreamID`-style ID prefix. There is no unit test here: it needs a live Wails
context. `go build` and the existing suites are the gate, and the manual test
plan covers the behaviour.

- [ ] **Step 1: Add the registry to the App struct**

In `app.go`, beside the existing `llmStreams`/`llmMu` fields (currently lines
52-53):

```go
	shellRuns     map[string]context.CancelFunc // active shell runs, keyed by runId
	shellMu       sync.Mutex                    // guards shellRuns
```

And beside `a.llmStreams = map[string]context.CancelFunc{}` (currently line 65):

```go
	a.shellRuns = map[string]context.CancelFunc{}
```

- [ ] **Step 2: Cancel live runs when the app closes**

Find where `app:close-requested` is emitted in `app.go` (currently line 140).
Immediately BEFORE that emit, cancel every live run so quitting cannot orphan a
process:

```go
	a.shellMu.Lock()
	for _, cancel := range a.shellRuns {
		cancel()
	}
	a.shellMu.Unlock()
```

- [ ] **Step 3: Write `shell_methods.go`**

```go
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/shell"
)

// shellEmitInterval is how often buffered output is flushed to the frontend.
// Small enough to feel live, large enough that `find /` does not put one event
// per line across the bridge.
const shellEmitInterval = 50 * time.Millisecond

func newRunID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "shell-" + hex.EncodeToString(b)
}

// StartShellRunReq is what the cell sends to start a run.
type StartShellRunReq struct {
	ShellID string `json:"shellId"`
	Cwd     string `json:"cwd"`
	Command string `json:"command"`
}

// ListShells returns the shells installed on this machine, for the cell's
// dropdown. An empty slice is meaningful — the cell says so rather than
// showing an empty picker.
func (a *App) ListShells() []shell.Shell {
	found := shell.Discover(shell.OSEnv())
	if found == nil {
		return []shell.Shell{}
	}
	return found
}

// StartShellRun runs a command in a goroutine, emitting shell:out / shell:exit /
// shell:error keyed by the returned runId. Mirrors StartLLMTurn.
//
// There is deliberately no timeout: `docker build` and `apt upgrade` take
// minutes. Cancellation is the user's Stop button, and output volume is capped
// instead of wall-clock time.
func (a *App) StartShellRun(req StartShellRunReq) (string, error) {
	var sh shell.Shell
	for _, candidate := range shell.Discover(shell.OSEnv()) {
		if candidate.ID == req.ShellID {
			sh = candidate
			break
		}
	}
	if sh.ID == "" {
		return "", errors.New("shell not available: " + req.ShellID)
	}

	runID := newRunID()
	ctx, cancel := context.WithCancel(context.Background())

	a.shellMu.Lock()
	a.shellRuns[runID] = cancel
	a.shellMu.Unlock()

	batcher := shell.NewBatcher(64 << 10)
	flush := func() {
		if chunks := batcher.Take(); len(chunks) > 0 {
			runtime.EventsEmit(a.ctx, "shell:out", map[string]any{
				"runId": runID, "chunks": chunks,
			})
		}
	}

	go func() {
		ticker := time.NewTicker(shellEmitInterval)
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-ticker.C:
					flush()
				case <-stop:
					return
				}
			}
		}()

		defer func() {
			ticker.Stop()
			close(stop)
			flush() // whatever arrived since the last tick
			a.shellMu.Lock()
			delete(a.shellRuns, runID)
			a.shellMu.Unlock()
			cancel()
		}()

		res, err := shell.Run(ctx, sh, req.Cwd, req.Command, batcher.Add)
		if err != nil && ctx.Err() == nil {
			runtime.EventsEmit(a.ctx, "shell:error", map[string]string{
				"runId": runID, "message": err.Error(),
			})
			return
		}
		// A cancelled run reports -1 so the cell can say "cancelled" rather than
		// inventing an exit code the process never returned.
		exitCode := res.ExitCode
		if ctx.Err() != nil {
			exitCode = -1
		}
		runtime.EventsEmit(a.ctx, "shell:exit", map[string]any{
			"runId": runID, "exitCode": exitCode, "durationMs": res.DurationMs,
		})
	}()

	return runID, nil
}

// CancelShellRun stops an in-flight run (the Stop button). Cancelling the
// context kills the child's whole process group; see internal/shell/procattr_*.
func (a *App) CancelShellRun(runID string) {
	a.shellMu.Lock()
	cancel := a.shellRuns[runID]
	a.shellMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
```

- [ ] **Step 4: Verify it builds on both platforms and nothing regressed**

Run: `go build ./... && GOOS=windows GOARCH=amd64 go build ./... && make test`
Expected: no build output; all Go tests pass.

- [ ] **Step 5: Regenerate the Wails bindings**

The frontend imports these methods from `frontend/wailsjs/go/main/App`, which is
tracked in git and generated by Wails.

Run: `wails generate module`
Expected: `frontend/wailsjs/go/main/App.js`, `App.d.ts` and `models.ts` gain
`ListShells`, `StartShellRun`, `CancelShellRun` and the `shell.Shell` model.

If `wails` is not on PATH, install it with
`go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0` and retry. Do not
hand-edit the generated files.

- [ ] **Step 6: Commit**

```bash
git add app.go shell_methods.go frontend/wailsjs
git commit -m "feat(shell): ListShells, StartShellRun and CancelShellRun bindings"
```

---

### Task 6: `shell-fact.js` — the N4L builder and sanitiser

**Files:**
- Create: `frontend/src/lib/shell-fact.js`
- Create: `frontend/src/lib/shell-fact.test.js`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `sanitizeTarget(line) -> string`
  - `buildShellFact({subject, command, lines}) -> string` — `''` when unusable.

**The format**, per `buildMediaFact` in `media-fact.js`:

```
<subject>
"  (cmd) <command>
"  (out) <line 1>
"  (out) <line 2>
```

**Why the sanitiser is narrow.** Each output line becomes the *target* of a
ditto line, so it never sits at the start of a line. That defuses the
leading-character hazards (`#`, `"`, `-`, `::`, `@`) automatically — the same
argument `media-fact.js` already makes for `@`: *"This line always begins with
the ditto prefix, so the `@` is ordinary text."* What remains genuinely
dangerous is **parentheses**, because `(...)` is arrow syntax wherever it
appears; `media-fact.js` records a parenthesised timestamp being silently
consumed by the parser. Task 7 proves these claims against the real Go parser.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/shell-fact.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildShellFact, sanitizeTarget } from './shell-fact.js'

test('a fact names the subject, the command, and one line per output line', () => {
  const got = buildShellFact({
    subject: 'check containers on prod',
    command: "docker ps --format '{{.Names}}'",
    lines: ['pg-primary', 'redis-cache'],
  })
  assert.equal(got,
    'check containers on prod\n' +
    '"  (cmd) docker ps --format \'{{.Names}}\'\n' +
    '"  (out) pg-primary\n' +
    '"  (out) redis-cache\n')
})

// (...) is arrow syntax wherever it appears in a target — media-fact.js records
// a parenthesised timestamp being silently eaten by the parser.
test('parentheses are replaced, never passed through', () => {
  assert.equal(sanitizeTarget('at foo(bar) line 3'), 'at foo[bar] line 3')
  assert.equal(sanitizeTarget('(note) not an arrow'), '[note] not an arrow')
  assert.ok(!sanitizeTarget('a(b)c').includes('('))
  assert.ok(!sanitizeTarget('a(b)c').includes(')'))
})

// These are safe only because the ditto prefix means they are never first on
// the line. Asserting it documents why no escaping is needed.
test('leading N4L markers pass through untouched', () => {
  assert.equal(sanitizeTarget('# not a comment here'), '# not a comment here')
  assert.equal(sanitizeTarget('-rw-r--r-- 1 root root'), '-rw-r--r-- 1 root root')
  assert.equal(sanitizeTarget(':: not a section'), ':: not a section')
  assert.equal(sanitizeTarget('@reboot /usr/bin/thing'), '@reboot /usr/bin/thing')
  assert.equal(sanitizeTarget('"quoted"'), '"quoted"')
})

test('tabs and carriage returns become spaces, and edges are trimmed', () => {
  assert.equal(sanitizeTarget('a\tb'), 'a b')
  assert.equal(sanitizeTarget('trailing\r'), 'trailing')
  assert.equal(sanitizeTarget('   padded   '), 'padded')
})

test('an empty or whitespace-only line sanitises to the empty string', () => {
  assert.equal(sanitizeTarget(''), '')
  assert.equal(sanitizeTarget('   '), '')
  assert.equal(sanitizeTarget('\t\r'), '')
  assert.equal(sanitizeTarget(null), '')
  assert.equal(sanitizeTarget(undefined), '')
})

test('an over-long line is capped at 400 characters', () => {
  const long = 'x'.repeat(900)
  assert.equal(sanitizeTarget(long), 'x'.repeat(400))
  assert.equal(sanitizeTarget('y'.repeat(400)), 'y'.repeat(400))
})

test('blank output lines are dropped rather than becoming empty facts', () => {
  const got = buildShellFact({ subject: 's', command: 'c', lines: ['a', '', '   ', 'b'] })
  assert.equal(got, 's\n"  (cmd) c\n"  (out) a\n"  (out) b\n')
})

test('at most 200 output lines are promoted', () => {
  const lines = Array.from({ length: 500 }, (_, i) => `line${i}`)
  const got = buildShellFact({ subject: 's', command: 'c', lines })
  assert.equal(got.split('(out)').length - 1, 200)
  assert.ok(got.includes('"  (out) line199\n'))
  assert.ok(!got.includes('"  (out) line200\n'))
})

// Mirrors buildMediaFact, which returns '' without a subject.
test('no subject means no fact', () => {
  assert.equal(buildShellFact({ subject: '', command: 'c', lines: ['a'] }), '')
  assert.equal(buildShellFact({ subject: '   ', command: 'c', lines: ['a'] }), '')
  assert.equal(buildShellFact({ command: 'c', lines: ['a'] }), '')
})

test('nothing to say means no fact', () => {
  assert.equal(buildShellFact({ subject: 's', command: '', lines: [] }), '')
  assert.equal(buildShellFact({ subject: 's', command: '   ', lines: ['  '] }), '')
  assert.equal(buildShellFact({ subject: 's' }), '')
  assert.equal(buildShellFact(), '')
  assert.equal(buildShellFact(null), '')
})

test('output alone, with no command, still promotes', () => {
  assert.equal(buildShellFact({ subject: 's', command: '', lines: ['a'] }), 's\n"  (out) a\n')
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `make test-js`
Expected: FAIL — `Cannot find module '.../frontend/src/lib/shell-fact.js'`.

- [ ] **Step 3: Write `frontend/src/lib/shell-fact.js`**

```js
// Builds the N4L text promoted from a shell cell: a fact about the command that
// produced the selected output. Pure functions, so they are unit-testable
// without mounting Svelte — the same reasoning media-fact.js records.
//
// Shape, matching buildMediaFact:
//
//   <subject>
//   "  (cmd) <command>
//   "  (out) <line>
//
// Arrows are referenced by short CODE, not long name. Declared in
// SSTconfig/arrows-EP-3.sst:
//   cmd = "has command"   out = "has output"
const COMMAND_ARROW = 'cmd'
const OUTPUT_ARROW = 'out'

// A promoted entry is a runbook line, not a log dump. 200 lines is generous for
// something a human will read, and it bounds what one click can append to a
// note file.
const MAX_LINES = 200

// Long enough for a real `ls -l` row or a stack frame, short enough that one
// runaway line cannot dominate the file.
const MAX_LINE = 400

// sanitizeTarget makes one line of shell output safe to use as the target of a
// ditto line.
//
// It is narrower than it looks. Because every target is preceded by the ditto
// prefix (`"  (out) `), it is never the first thing on a line — so N4L's
// leading-position markers (`#` comment, `"` ditto, `-` chapter, `::` section,
// `@` alias) are all ordinary text here and pass through untouched. This is the
// same argument media-fact.js makes for a leading `@`.
//
// Parentheses are the real hazard: `(...)` is arrow syntax WHEREVER it appears,
// and media-fact.js records a parenthesised timestamp being silently consumed
// by the parser and never reaching the graph. Square brackets keep the shape
// readable, which matters for a stack trace or a `docker ps` row — deliberately
// lossy rather than dropping the characters.
//
// Newlines are not handled here: the caller splits output into lines first, and
// one line is one fact.
export function sanitizeTarget(line) {
  let t = String(line ?? '').replace(/[\r\t\n]/g, ' ').trim()
  if (!t) return ''
  t = t.replace(/\(/g, '[').replace(/\)/g, ']')
  return t.length > MAX_LINE ? t.slice(0, MAX_LINE) : t
}

// buildShellFact returns the N4L for a promoted selection, or '' when there is
// nothing usable — as buildMediaFact returns '' without a subject. The caller
// inserts the result as an N4L draft cell, so the user reads and edits it
// before it ever reaches a file.
export function buildShellFact({ subject, command, lines } = {}) {
  const subj = String(subject ?? '').trim()
  if (!subj) return ''

  const cmd = sanitizeTarget(command)
  const outs = (Array.isArray(lines) ? lines : [])
    .map(sanitizeTarget)
    .filter(Boolean)
    .slice(0, MAX_LINES)

  if (!cmd && outs.length === 0) return ''

  let out = `${subj}\n`
  if (cmd) out += `"  (${COMMAND_ARROW}) ${cmd}\n`
  for (const line of outs) out += `"  (${OUTPUT_ARROW}) ${line}\n`
  return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS — the new file's tests plus every existing `lib/*.test.js`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/shell-fact.js frontend/src/lib/shell-fact.test.js
git commit -m "feat(shell): build the promoted N4L fact, and sanitise its targets"
```

---

### Task 7: Declare the arrows, and prove the sanitiser against the real parser

**Files:**
- Modify: `SSTconfig/arrows-EP-3.sst` (beside the `has picture` / `has video` block)
- Create: `internal/content/shell_target_test.go`

**Interfaces:**
- Consumes: the output shapes `sanitizeTarget` produces (Task 6).
- Produces: the `cmd` and `out` arrow codes, and proof that promoted facts parse.

**Why this task exists.** `sanitizeTarget` is JavaScript; the parser is Go. A
`node:test` can only assert the sanitiser's output *shape*, never that Go
accepts it. `media-fact.js` closed this gap with a hand-written comment
(*"Verified: a note target survives intact with @ 3:43"*). This makes it a real
test, so the claim cannot rot.

Codes were checked against every existing declaration before choosing. **Do not
use `ran`** — it sits confusingly beside the existing `runs`/`runafter`/
`runbefore`/`runsin`, which is the collision class recorded in commit `582ca30`.

- [ ] **Step 1: Declare the arrows**

In `SSTconfig/arrows-EP-3.sst`, in the `:: attributes, supporting information ::`
block, directly after the `+ has video (vid) - is video for (vid-for)` line.
Match the surrounding alignment exactly — a leading space, then `+`:

```
 + has command      (cmd)          - was run for          (runfor)
 + has output       (out)          - is the output of     (outof)
```

- [ ] **Step 2: Write the failing test**

Create `internal/content/shell_target_test.go`. The API used below is this
package's real one: `ParseFile(path, *SSTRegistry) (*ParsedFile, []ParseError)`,
and the parsed shape is `ParsedFile.Chapters[i].Nodes[j].Dittos[k].Arrows[m]`
with `Arrow{Code, Target, IsRegistered}` (see `internal/content/types.go`).
Because `Arrow.Target` is exposed directly, the assertion can compare the target
*exactly* rather than searching a dump.

```go
package content

import (
	"os"
	"path/filepath"
	"testing"
)

// shellReg is a minimal registry declaring just the arrows a promoted shell
// fact uses, mirroring how parser_test.go builds registries for its fixtures.
func shellReg() *SSTRegistry {
	return &SSTRegistry{
		Arrows: map[string]ArrowDef{
			"cmd": {Code: "cmd", LongName: "has command", MetaType: "properties"},
			"out": {Code: "out", LongName: "has output", MetaType: "properties"},
		},
		Annotations: map[string]AnnotDef{},
	}
}

// writeN4L writes one promoted-shell-fact fixture and returns its path.
func writeN4L(t *testing.T, target string) string {
	t.Helper()
	src := "- shell\n\n:: runbook ::\n\ncheck containers\n\"  (out) " + target + "\n"
	path := filepath.Join(t.TempDir(), "shell.n4l")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// firstDittoArrow returns the single arrow the fixture declares.
func firstDittoArrow(t *testing.T, pf *ParsedFile) Arrow {
	t.Helper()
	if len(pf.Chapters) == 0 || len(pf.Chapters[0].Nodes) == 0 {
		t.Fatalf("no nodes parsed")
	}
	node := pf.Chapters[0].Nodes[0]
	if len(node.Dittos) == 0 || len(node.Dittos[0].Arrows) == 0 {
		t.Fatalf("no ditto arrow parsed from node %q", node.Text)
	}
	return node.Dittos[0].Arrows[0]
}

// Shell output promoted into N4L arrives as the TARGET of a ditto line. This is
// the Go-side proof of a claim made in JavaScript: that sanitizeTarget's output
// survives this parser with the target intact.
//
// frontend/src/lib/shell-fact.js argues that N4L's leading-position markers are
// safe in a target because the ditto prefix means they are never first on the
// line, and that parentheses are the one real hazard. If either claim is wrong,
// this test fails — which is the point. media-fact.js could only assert it in a
// comment.
func TestShellPromotedTargetsSurviveTheParser(t *testing.T) {
	// Each target is what sanitizeTarget would emit for real shell output.
	cases := []struct {
		name   string
		target string
	}{
		{"plain", "pg-primary"},
		{"ls long form", "-rw-r--r-- 1 root root 4096 Jul 30 10:12 hosts"},
		{"leading hash", "# not a comment here"},
		{"leading colons", ":: not a section"},
		{"leading at", "@reboot /usr/bin/thing"},
		{"docker go template", "docker ps --format '{{.Names}}'"},
		{"brackets from sanitised parens", "at foo[bar] line 3"},
		{"quotes", `he said "hello"`},
		{"path with slashes", "/var/log/nginx/access.log"},
		{"equals and ampersand", "ENV=prod && echo done"},
		{"unicode", "café → ok"},
		{"percent and star", "cpu 12.5% ***"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf, _ := ParseFile(writeN4L(t, tc.target), shellReg())
			got := firstDittoArrow(t, pf)
			if got.Code != "out" {
				t.Fatalf("arrow code = %q, want %q", got.Code, "out")
			}
			if !got.IsRegistered {
				t.Fatalf("arrow %q should be registered by shellReg()", got.Code)
			}
			if got.Target != tc.target {
				t.Fatalf("target did not survive:\n got %q\nwant %q", got.Target, tc.target)
			}
		})
	}
}

// The hazard sanitizeTarget exists to remove: a raw parenthesised target. This
// documents the actual failure mode so nobody "simplifies" the sanitiser away.
//
// The assertion is deliberately weak — it asserts only that a raw target with
// parentheses does NOT round-trip identically, which is what justifies
// sanitising. It does not pin down how the parser mangles it, since that is the
// parser's business and not a contract this test should freeze.
func TestRawParensDoNotRoundTrip(t *testing.T) {
	raw := "at foo(bar) line 3"
	pf, _ := ParseFile(writeN4L(t, raw), shellReg())
	got := firstDittoArrow(t, pf)
	if got.Target == raw {
		t.Fatalf("raw parens round-tripped intact (%q) — if the parser now preserves "+
			"them, sanitizeTarget's paren replacement is no longer required and both "+
			"this test and that comment should be revisited", got.Target)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/content/ -run 'TestShellPromotedTargets|TestRawParens' -v`
Expected: FAIL — the file does not exist yet, so the package has no such tests
(`no tests to run`), and once added it must fail only if a claim is wrong.

- [ ] **Step 4: Make it pass**

The test needs no production code — the arrows from Step 1 and the existing
parser are the implementation. Run it:

Run: `go test ./internal/content/ -v`
Expected: PASS, including every subtest.

**If a target does not survive, that is a real finding, not a test to weaken.**
Fix `sanitizeTarget` in `frontend/src/lib/shell-fact.js`, add the matching case
to `shell-fact.test.js`, record it in the sanitiser's comment, and re-run. Never
delete a case from the table to get green.

- [ ] **Step 5: Verify the arrow file still loads and nothing regressed**

Run: `make test && make test-js`
Expected: all pass. A malformed arrow declaration surfaces here if any parser
test loads `SSTconfig`.

- [ ] **Step 6: Commit**

```bash
git add SSTconfig/arrows-EP-3.sst internal/content/shell_target_test.go
git commit -m "feat(config): declare has command/has output arrows; prove promoted targets parse"
```

---

### Task 8: `shell-run.js` — the streaming host and the scrollback buffer

**Files:**
- Create: `frontend/src/lib/shell-run.js`
- Create: `frontend/src/lib/shell-run.test.js`

**Interfaces:**
- Consumes: `StartShellRun`, `CancelShellRun` from
  `../../wailsjs/go/main/App`; `EventsOn` from `../../wailsjs/runtime/runtime`.
- Produces:
  - `appendChunks(buffer, chunks, maxLines) -> Array` — pure, tested.
  - `runShell({shellId, cwd, command, onChunks, onExit, onError}) -> {cancel}`

`runShell` mirrors `streamTurn` in `frontend/src/lib/llm.js:61-86` and
`runCompute` in `compute-host.js`: it returns `{cancel}`, filters events by ID,
and unsubscribes on completion. Like `llm.js`, it has no unit test — it is
Wails wiring. `appendChunks` holds the logic worth testing and is exported
separately for that reason.

**The race to replicate.** `streamTurn` sets a `cancelled` flag before the
`StartLLMTurn` promise resolves, and cancels immediately on resolution if it was
set. Without that, clicking Stop in the moment between Run and the backend
returning a run ID leaves the process running with nothing tracking it. Copy
this.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/shell-run.test.js`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { appendChunks } from './shell-run.js'

test('chunks are split into lines and appended in order', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\nb\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', 'b'])
  assert.deepEqual(got.map((l) => l.stream), ['stdout', 'stdout'])
})

// Output arrives mid-line: a chunk boundary is not a line boundary.
test('a partial line is completed by the next chunk', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'hel' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['hel'])
  buf = appendChunks(buf, [{ stream: 'stdout', text: 'lo\n' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['hello'])
})

// A partial stdout line must not swallow a stderr chunk that arrives next.
test('a partial line is only continued by the same stream', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'out-partial' }], 100)
  buf = appendChunks(buf, [{ stream: 'stderr', text: 'err\n' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['out-partial', 'err'])
  assert.deepEqual(buf.map((l) => l.stream), ['stdout', 'stderr'])
})

test('stderr lines keep their stream tag so the UI can tint them', () => {
  const got = appendChunks([], [{ stream: 'stderr', text: 'boom\n' }], 100)
  assert.equal(got[0].stream, 'stderr')
})

test('carriage returns are stripped so CRLF output does not gain blank lines', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\r\nb\r\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', 'b'])
})

// Without this, `yes` exhausts the webview.
test('the buffer is capped, dropping the oldest lines', () => {
  let buf = []
  for (let i = 0; i < 20; i++) buf = appendChunks(buf, [{ stream: 'stdout', text: `${i}\n` }], 5)
  assert.equal(buf.length, 5)
  assert.deepEqual(buf.map((l) => l.text), ['15', '16', '17', '18', '19'])
})

test('a single oversized chunk is also capped', () => {
  const text = Array.from({ length: 500 }, (_, i) => `l${i}`).join('\n') + '\n'
  const got = appendChunks([], [{ stream: 'stdout', text }], 10)
  assert.equal(got.length, 10)
  assert.equal(got[9].text, 'l499')
})

test('empty and malformed input leaves the buffer alone', () => {
  const buf = [{ stream: 'stdout', text: 'keep' }]
  assert.deepEqual(appendChunks(buf, [], 100), buf)
  assert.deepEqual(appendChunks(buf, null, 100), buf)
  assert.deepEqual(appendChunks(buf, [{ stream: 'stdout', text: '' }], 100), buf)
})

test('blank lines within output are preserved', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\n\nb\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', '', 'b'])
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `make test-js`
Expected: FAIL — `Cannot find module '.../frontend/src/lib/shell-run.js'`.

- [ ] **Step 3: Write `frontend/src/lib/shell-run.js`**

```js
import { StartShellRun, CancelShellRun } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// Scrollback lives here, in memory, and is never persisted: SaveNotebook writes
// every cell as one JSON blob on a debounce, so keeping megabytes of `docker
// logs` output on the cell would rewrite the whole notebook repeatedly. The
// promoted N4L is the durable record; this buffer is scratch.
export const MAX_LINES = 5000

// appendChunks folds tagged chunks into a line buffer.
//
// A chunk boundary is not a line boundary — the backend reads fixed-size blocks,
// so a line routinely arrives in pieces. The tail line stays "open" until a
// newline closes it, but only continues if the next chunk is the SAME stream;
// otherwise a partial stdout line would swallow the stderr text that follows.
//
// Returns a new array (Svelte reactivity needs the assignment), capped to
// maxLines by dropping the oldest — without which `yes` exhausts the webview.
export function appendChunks(buffer, chunks, maxLines = MAX_LINES) {
  if (!Array.isArray(chunks) || chunks.length === 0) return buffer

  let out = buffer
  let copied = false
  const ensureCopy = () => {
    if (!copied) { out = buffer.slice(); copied = true }
  }

  for (const chunk of chunks) {
    const text = String(chunk?.text ?? '').replace(/\r/g, '')
    if (!text) continue
    ensureCopy()

    const stream = chunk?.stream === 'stderr' ? 'stderr' : 'stdout'
    const parts = text.split('\n')
    const tail = out.length ? out[out.length - 1] : null

    // parts[0] continues an open line of the same stream, if there is one.
    if (tail && tail.open && tail.stream === stream) {
      out[out.length - 1] = { ...tail, text: tail.text + parts[0] }
    } else if (parts[0] !== '' || parts.length === 1) {
      out.push({ stream, text: parts[0], open: true })
    }

    for (let i = 1; i < parts.length; i++) {
      // The line before each newline is now closed.
      if (out.length) out[out.length - 1] = { ...out[out.length - 1], open: false }
      const isLast = i === parts.length - 1
      // A trailing newline leaves nothing open; anything after it starts a new
      // open line.
      if (!isLast || parts[i] !== '') out.push({ stream, text: parts[i], open: true })
    }
  }

  if (copied && out.length > maxLines) out = out.slice(out.length - maxLines)
  return out
}

// runShell starts a run and wires the out/exit/error events, filtered to this
// run's id. Returns a cancel() that stops the run and unsubscribes.
//
// The `cancelled` flag handles Stop being pressed before StartShellRun resolves
// — without it the process would keep running with nothing tracking its id.
// Copied from streamTurn in llm.js, which solves the same race.
export function runShell({ shellId, cwd, command, onChunks, onExit, onError }) {
  let offOut = () => {}
  let offExit = () => {}
  let offError = () => {}
  let rid = null
  let cancelled = false

  StartShellRun({ shellId: shellId || '', cwd: cwd || '', command })
    .then((runId) => {
      rid = runId
      if (cancelled) { CancelShellRun(rid); return }
      offOut = EventsOn('shell:out', (p) => { if (p?.runId === rid) onChunks(p.chunks ?? []) })
      offExit = EventsOn('shell:exit', (p) => {
        if (p?.runId === rid) { cleanup(); onExit(p.exitCode, p.durationMs) }
      })
      offError = EventsOn('shell:error', (p) => {
        if (p?.runId === rid) { cleanup(); onError(p.message) }
      })
    })
    .catch((e) => onError(String(e?.message ?? e)))

  function cleanup() { offOut(); offExit(); offError() }
  function cancel() { cancelled = true; if (rid) CancelShellRun(rid); cleanup() }
  return { cancel }
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS — all nine new tests plus every existing `lib/*.test.js`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/shell-run.js frontend/src/lib/shell-run.test.js
git commit -m "feat(shell): stream a run into a capped line buffer"
```

---

### Task 9: `ShellCell.svelte`, and registering the cell type

**Files:**
- Create: `frontend/src/components/notebook/ShellCell.svelte`
- Modify: `frontend/src/lib/notebook.js` (import, `newShellCell`, `cellRegistry`)
- Modify: `frontend/src/lib/cell-label.js` (a `CELL_NAMES` entry and a detail arm)
- Modify: `frontend/src/lib/cell-label.test.js` (tests for both)
- Modify: `frontend/src/lib/cell-preview.js` (a `shell` arm)
- Modify: `frontend/src/lib/cell-preview.test.js` (tests for it)
- Modify: `frontend/src/components/notebook/NotebookPane.svelte` (an `addShell` handler and a `+ Shell` button)

**Interfaces:**
- Consumes: `runShell`, `appendChunks`, `MAX_LINES` (Task 8); `ListShells` from
  `../../wailsjs/go/main/App` (Task 5).
- Produces: a `shell` cell whose persisted fields are
  `{id, type: 'shell', shell, shellLabel, cwd, command, subject}`. Task 10 adds
  the promote bar to this component.

**Why `shellLabel` is persisted alongside `shell`.** `cellLabel` is a pure
function with no access to the shell list, and `cell.shell` holds an ID like
`wsl:Ubuntu`. Persisting the human label too lets the header read
`Shell · Ubuntu (WSL)` instead of `Shell · wsl:Ubuntu`, without making
`cellLabel` impure or async.

- [ ] **Step 1: Write the failing tests for the two pure modules**

In `frontend/src/lib/cell-label.test.js`, add:

```js
test('a shell cell is named, with the chosen shell as its detail', () => {
  assert.equal(cellLabel({ type: 'shell', shellLabel: 'Ubuntu (WSL)' }), 'Shell · Ubuntu (WSL)')
  assert.equal(cellLabel({ type: 'shell', shellLabel: 'bash' }), 'Shell · bash')
})

// A cell created before a shell was picked must still name itself.
test('a shell cell with no shell chosen shows no dangling separator', () => {
  assert.equal(cellLabel({ type: 'shell' }), 'Shell')
  assert.equal(cellLabel({ type: 'shell', shellLabel: '' }), 'Shell')
})
```

In `frontend/src/lib/cell-preview.test.js`, add:

```js
test('a shell cell previews its command', () => {
  assert.equal(cellPreview({ type: 'shell', command: 'docker ps' }), 'docker ps')
})

test('a multi-line shell command previews its first non-empty line', () => {
  assert.equal(cellPreview({ type: 'shell', command: '\n\ncd /tmp\nls -la' }), 'cd /tmp')
})

test('a shell cell with no command yet previews as the empty string', () => {
  assert.equal(cellPreview({ type: 'shell', command: '' }), '')
  assert.equal(cellPreview({ type: 'shell' }), '')
})
```

- [ ] **Step 2: Run them to verify they fail**

Run: `make test-js`
Expected: FAIL — `cellLabel` returns `'shell'` (its raw-type fallback) and
`cellPreview` returns `''` for a command-bearing cell.

- [ ] **Step 3: Add the two arms**

In `frontend/src/lib/cell-label.js`, add to `CELL_NAMES`:

```js
  shell: 'Shell',
```

and in `cellLabel`, alongside the existing detail arms:

```js
  else if (type === 'shell') detail = cell.shellLabel || ''
```

In `frontend/src/lib/cell-preview.js`, add to the `switch` in `cellPreview`:

```js
    case 'shell': return firstLine(cell.command)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `make test-js`
Expected: PASS.

- [ ] **Step 5: Add the cell factory and registry entry**

In `frontend/src/lib/notebook.js`, add the import beside the others:

```js
import ShellCell from '../components/notebook/ShellCell.svelte'
```

the factory beside the others:

```js
// Shell cell: runs one command in a chosen OS shell. `shell`/`shellLabel` are
// the dropdown's id and display text; scrollback is deliberately absent, since
// it is never persisted. See the shell-cell design doc.
export function newShellCell() {
  seq += 1
  return {
    id: `cell-${Date.now()}-${seq}`, type: 'shell',
    shell: '', shellLabel: '', cwd: '', command: '', subject: '',
  }
}
```

and the registry entry:

```js
  shell: ShellCell,
```

- [ ] **Step 6: Write `ShellCell.svelte`**

Create `frontend/src/components/notebook/ShellCell.svelte`. Match the visual
language of `ComputeCell.svelte` — read it first for the bar/output/error
structure and the exact colours; reuse its palette rather than inventing one.

```svelte
<script>
  import { onDestroy, onMount } from 'svelte'
  import { ListShells } from '../../../wailsjs/go/main/App'
  import { runShell, appendChunks } from '../../lib/shell-run.js'

  export let cell
  export let onChange = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused
  export let onInsert = () => {}

  let shells = []
  let discoverErr = ''
  let lines = []          // scrollback — component-local, never persisted
  let running = false
  let run = null
  let status = ''         // "exit 0 · 0.4s", "cancelled", or a spawn error
  let statusBad = false

  onMount(async () => {
    try {
      shells = await ListShells()
      if (!shells.length) { discoverErr = 'No shell found on this system.'; return }
      // Preselect the first discovered shell, or restore a saved choice that is
      // still installed. A saved shell that has gone away must not silently
      // become a different one.
      const saved = shells.find((s) => s.id === cell.shell)
      if (!saved) selectShell(shells[0])
      else if (saved.label !== cell.shellLabel) selectShell(saved)
    } catch (e) { discoverErr = String(e?.message ?? e) }
  })

  onDestroy(() => run?.cancel())

  function selectShell(s) {
    cell.shell = s.id
    cell.shellLabel = s.label
    onChange(cell)
  }
  function onPickShell(e) {
    const s = shells.find((x) => x.id === e.target.value)
    if (s) selectShell(s)
  }
  function onCwd(e) { cell.cwd = e.target.value; onChange(cell) }
  function onCommand(e) { cell.command = e.target.value; onChange(cell) }

  // Ctrl/Cmd+Enter runs, matching LLMCell's send shortcut.
  function onKey(e) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); start() }
  }

  function start() {
    if (readOnly || running || !cell.command?.trim() || !cell.shell) return
    lines = []
    status = ''
    statusBad = false
    running = true
    run = runShell({
      shellId: cell.shell, cwd: cell.cwd, command: cell.command,
      onChunks: (chunks) => { lines = appendChunks(lines, chunks) },
      onExit: (code, ms) => {
        running = false; run = null
        // A non-zero exit is information, not a failure: it belongs here, not in
        // an error toast. `grep` returning 1 is normal.
        status = code === -1 ? 'cancelled' : `exit ${code} · ${(ms / 1000).toFixed(1)}s`
        statusBad = code !== 0
      },
      onError: (message) => {
        running = false; run = null
        status = message; statusBad = true
      },
    })
  }
  function stop() { run?.cancel(); run = null; running = false; status = 'cancelled'; statusBad = true }
</script>

<div class="sh-cell">
  {#if discoverErr}
    <div class="sh-error">{discoverErr}</div>
  {:else}
    {#if !readOnly}
      <div class="sh-bar">
        <select class="sh-shell" value={cell.shell} on:change={onPickShell} disabled={running}>
          {#each shells as s (s.id)}<option value={s.id}>{s.label}</option>{/each}
        </select>
        <input class="sh-cwd" type="text" placeholder="working directory"
          value={cell.cwd ?? ''} on:input={onCwd} disabled={running} />
        <span class="sh-spacer"></span>
        {#if running}
          <button class="sh-stop" on:click={stop}>Stop</button>
        {:else}
          <button class="sh-run" on:click={start} disabled={!cell.command?.trim()}>Run</button>
        {/if}
      </div>
    {/if}

    <textarea class="sh-cmd" value={cell.command ?? ''} on:input={onCommand} on:keydown={onKey}
      disabled={readOnly} spellcheck="false" rows="2"
      placeholder={'docker ps --format \'{{.Names}}\'\nCtrl+Enter to run'}></textarea>

    {#if lines.length || status}
      <div class="sh-out">
        {#each lines as l, i (i)}<div class="sh-line" class:err={l.stream === 'stderr'}>{l.text}</div>{/each}
        {#if status}<div class="sh-status" class:bad={statusBad}>{status}</div>{/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .sh-cell { border: 1px solid #2a3a4a; border-radius: 8px; background: #0e1626; overflow: hidden; }
  .sh-bar { display: flex; align-items: center; gap: 6px; padding: 5px 8px; border-bottom: 1px solid #2a3a4a; }
  .sh-shell, .sh-cwd { background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 6px; font-size: 12px; }
  .sh-cwd { flex: 1; min-width: 0; }
  .sh-spacer { flex: none; }
  .sh-run, .sh-stop { background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 10px; cursor: pointer; font-size: 12px; }
  .sh-run:disabled { opacity: .45; cursor: default; }
  .sh-stop { color: #ff8080; border-color: #5a2a2a; }
  .sh-cmd { width: 100%; box-sizing: border-box; background: #0b1220; color: #e8eaf0; border: none; border-bottom: 1px solid #2a3a4a; padding: 6px 8px; font-family: ui-monospace, monospace; font-size: 12px; resize: vertical; }
  .sh-out { max-height: 320px; overflow: auto; padding: 6px 8px; font-family: ui-monospace, monospace; font-size: 12px; }
  .sh-line { white-space: pre-wrap; word-break: break-word; color: #c9d4e3; }
  .sh-line.err { color: #ffb4a0; }
  .sh-status { margin-top: 4px; color: #5d7186; font-size: 11px; }
  .sh-status.bad { color: #ff8080; }
  .sh-error { padding: 8px; color: #ff8080; font-size: 12px; }
</style>
```

- [ ] **Step 7: Add the `+ Shell` button**

In `NotebookPane.svelte`, add `newShellCell` to the existing import from
`../../lib/notebook.js`, then the handler beside the other `add*` functions:

```js
  function addShell() { cells = [...cells, newShellCell()]; scheduleSave() }
```

and the button after `+ Media` in the `{#if !readOnly}` block:

```svelte
      <button class="nb-add" on:click={addShell}>+ Shell</button>
```

- [ ] **Step 8: Verify the build and tests**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS; build succeeds. A Svelte "unused CSS selector" warning
means a class in the `<style>` block is not reachable in the markup — fix the
markup, do not delete the rule blindly.

- [ ] **Step 9: Verify by hand**

Run `make dev`, open a notebook, click `+ Shell`, and confirm:

1. The dropdown lists real shells from your machine; the header reads
   `Shell · <label>`.
2. `echo hello` runs and prints `hello`, then `exit 0 · 0.0s`.
3. `echo oops 1>&2` shows the text tinted differently from stdout.
4. `exit 3` shows `exit 3` in red and **no error toast**.
5. `sleep 30` then Stop returns promptly and reads `cancelled`. In another
   terminal, `ps -ef | grep sleep` shows nothing left behind.
6. `cat` returns immediately rather than hanging — stdin is closed.
7. Setting the working directory to `/tmp` and running `pwd` prints `/tmp`;
   a nonexistent directory gives a readable error before anything runs.
8. Reopen the notebook: shell, cwd and command are still there, and the output
   pane is empty — scrollback is not persisted, by design.
9. Open the notebook read-only: no dropdown, no cwd field, no Run button, and
   the command textarea is disabled.
10. Fold the cell mid-run (`sleep 10; echo done`), unfold: still running, and
    `done` still arrives.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/components/notebook/ShellCell.svelte frontend/src/lib/notebook.js \
  frontend/src/lib/cell-label.js frontend/src/lib/cell-label.test.js \
  frontend/src/lib/cell-preview.js frontend/src/lib/cell-preview.test.js \
  frontend/src/components/notebook/NotebookPane.svelte
git commit -m "feat(notebook): shell cell — run a command, stream the output"
```

---

### Task 10: Promote a selection to N4L

**Files:**
- Modify: `frontend/src/components/notebook/ShellCell.svelte` (a selection handler, a promote bar, and their styles)

**Interfaces:**
- Consumes: `buildShellFact` from `../../lib/shell-fact.js` (Task 6); the
  existing `onInsert(text)` prop, which inserts an N4L draft cell after this one.
- Produces: nothing later tasks rely on.

This is the point of the feature. `TranscribeBar` and `MediaCell` are the
precedents: the user supplies a subject, and the promoted text lands in an N4L
**draft cell** they read and edit before it ever reaches a file.

- [ ] **Step 1: Capture the selection**

Add to `ShellCell.svelte`'s script:

```js
  import { buildShellFact } from '../../lib/shell-fact.js'

  let outEl                 // the output pane, for scoping the selection
  let selectedLines = []    // the lines currently selected, in order

  // No xterm means the output pane is plain DOM, so the browser's own selection
  // is all that is needed — no addon, no terminal API.
  //
  // The selection is intersected with each rendered line rather than read as one
  // string, so a multi-line selection promotes as separate facts, and a
  // selection that strays outside the pane contributes nothing.
  function readSelection() {
    const sel = window.getSelection()
    if (!outEl || !sel || sel.isCollapsed || sel.rangeCount === 0) { selectedLines = []; return }
    const range = sel.getRangeAt(0)
    if (!outEl.contains(range.commonAncestorContainer)) { selectedLines = []; return }
    selectedLines = Array.from(outEl.querySelectorAll('.sh-line'))
      .filter((el) => range.intersectsNode(el))
      .map((el) => el.textContent)
  }

  function promote() {
    const text = buildShellFact({
      subject: cell.subject, command: cell.command, lines: selectedLines,
    })
    if (!text) return
    onInsert(text)
  }

  function onSubject(e) { cell.subject = e.target.value; onChange(cell) }
```

- [ ] **Step 2: Wire the pane and add the promote bar**

Give the output container the binding and the selection listeners — `mouseup`
for dragging, `keyup` for shift-arrow selection:

```svelte
      <div class="sh-out" bind:this={outEl} on:mouseup={readSelection} on:keyup={readSelection}>
```

Then, after the `.sh-out` block and still inside the `{:else}`, add the promote
bar. It appears only when there is a selection, so it never occupies space
during a run:

```svelte
    {#if !readOnly && selectedLines.length}
      <div class="sh-promote">
        <span class="sh-count">{selectedLines.length} line{selectedLines.length === 1 ? '' : 's'}</span>
        <input class="sh-subj" type="text" placeholder="subject"
          value={cell.subject ?? ''} on:input={onSubject} />
        <button class="sh-n4l" on:click={promote} disabled={!cell.subject?.trim()}
          title="Promote the selection to an N4L draft cell">To N4L</button>
      </div>
    {/if}
```

- [ ] **Step 3: Add the styles**

```css
  .sh-promote { display: flex; align-items: center; gap: 6px; padding: 5px 8px; border-top: 1px solid #2a3a4a; background: #101826; }
  .sh-count { color: #5d7186; font-size: 11px; white-space: nowrap; }
  .sh-subj { flex: 1; min-width: 0; background: #14233a; color: #e8eaf0; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 6px; font-size: 12px; }
  .sh-n4l { background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 10px; cursor: pointer; font-size: 12px; white-space: nowrap; }
  .sh-n4l:disabled { opacity: .45; cursor: default; }
```

- [ ] **Step 4: Verify the build and tests**

Run: `make test-js && cd frontend && npx vite build`
Expected: tests PASS; build succeeds, with no unused-CSS-selector warning.

- [ ] **Step 5: Verify by hand**

Run `make dev` and confirm:

1. Run `docker ps` (or `ls -l /`). Select two output lines with the mouse — the
   promote bar appears and reads `2 lines`.
2. `To N4L` is disabled until a subject is typed.
3. Type a subject and click `To N4L`: an N4L draft cell appears **directly
   below** the shell cell, containing the subject, one `(cmd)` line, and one
   `(out)` line per selected line.
4. Push that draft cell to the file and confirm it parses — no error, and the
   text appears in the graph after Ctrl+G.
5. Select output containing parentheses (e.g. `python -c "raise ValueError('x')"`)
   and promote it. The `(` and `)` are `[` and `]` in the draft.
6. Click once in the output pane to collapse the selection: the promote bar
   disappears.
7. Select text, then drag a selection that starts inside the pane and ends
   outside it: nothing is promoted from outside the pane.
8. A read-only notebook shows no promote bar even with a selection.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/notebook/ShellCell.svelte
git commit -m "feat(shell): promote a selected part of the output to N4L"
```

---

### Task 11: Manual test plan document

**Files:**
- Create: `docs/superpowers/specs/2026-07-30-shell-cell-manual-test-plan.md`

**Interfaces:**
- Consumes: the behaviour built in Tasks 1-10.
- Produces: nothing consumed by code.

Every prior slice in this feature area shipped a manual test plan, because there
is no Svelte component harness and the interesting behaviour is in the
components. This one additionally covers the process-leak and
promote-parses-cleanly cases, which unit tests can only partly reach.

- [ ] **Step 1: Write the document**

Create `docs/superpowers/specs/2026-07-30-shell-cell-manual-test-plan.md`:

```markdown
# Shell cell — manual test plan

**Design:** `2026-07-30-shell-cell-design.md`
**Build:** `make dev`

Automated coverage: `internal/shell` (discovery, invocation args, streaming,
exit codes, cwd validation, stdin closed, process-group kill, batching) via
`make test`; `shell-fact.js` and `shell-run.js` via `make test-js`; promoted
target shapes against the real N4L parser in
`internal/content/shell_target_test.go`. Everything below is what those cannot
reach.

Prereqs: a collection with a processing notebook that can be edited, and the
ability to reopen one read-only.

## TC-1 The dropdown lists real shells

Add a `+ Shell` cell.

**Expect:** the dropdown lists shells actually installed on this machine — on
Linux your `$SHELL` first, then bash/zsh/sh without duplicates. The cell header
reads `Shell · <label>`. No entry appears twice.

## TC-2 A command runs and streams

Run `echo hello`.

**Expect:** `hello` in the output pane, then `exit 0 · 0.0s` in muted text.

Then run something slow and chatty — `for i in $(seq 1 50); do echo $i; sleep 0.05; done`.

**Expect:** lines appear progressively, not all at the end.

## TC-3 stderr is distinguished but stays in place

Run `echo out; echo err 1>&2; echo out2`.

**Expect:** all three lines present; the `err` line is tinted differently. Its
position relative to the stdout lines may vary between runs — that is expected
and documented, not a bug.

## TC-4 A non-zero exit is information, not an error

Run `exit 3`. Then run `grep nonexistent /etc/hostname`.

**Expect:** `exit 3` and `exit 1` shown in the output pane, tinted as bad.
**No error toast appears.** The cell is not in an error state.

## TC-5 Stop leaves nothing behind

Run `sleep 300 | cat`. Click **Stop**.

**Expect:** the run ends promptly and the status reads `cancelled`.

Then, in a separate terminal: `ps -ef | grep -E 'sleep 300' | grep -v grep`.

**Expect:** no output. Nothing survived.

*(This is the case `internal/shell/cancel_test.go` covers in Go; repeat it here
because the Windows path uses `taskkill` and has no automated test.)*

## TC-6 stdin is closed, so prompts fail rather than hang

Run `cat`.

**Expect:** returns immediately with `exit 0` — it read EOF, it did not hang.

Run `sudo -k true` (on a machine where sudo needs a password).

**Expect:** it fails with a readable error within a second or two. It does
**not** hang the cell.

## TC-7 Working directory

Set the working directory to `/tmp` and run `pwd`.

**Expect:** `/tmp`.

Set it to `/definitely/not/here` and run `echo hi`.

**Expect:** a readable error naming the directory, and **no output** — it failed
before spawning.

Reopen the notebook.

**Expect:** the working directory is still set.

## TC-8 What persists and what does not

Set a shell, a working directory and a command; run it so there is output. Close
the notebook and reopen it.

**Expect:** shell, working directory, command and subject are all restored. The
**output pane is empty** — scrollback is deliberately never persisted.

## TC-9 Read-only

Open a notebook containing a shell cell read-only.

**Expect:** no dropdown, no working-directory field, **no Run button**, no
promote bar. The command textarea is disabled. The command text is still
readable.

## TC-10 Folding does not interrupt a run

Run `sleep 10; echo done`. While it is running, fold the cell from its header.
Wait, then unfold.

**Expect:** still running when unfolded, and `done` arrives. Folding a shell
cell does not kill its run.

*(Folded cells stay mounted by design — see the 2026-07-29 folding design.)*

## TC-11 Promote a selection

Run `ls -l /`. Select three output lines with the mouse.

**Expect:** the promote bar appears, reading `3 lines`. `To N4L` is disabled
until a subject is typed.

Type a subject and click `To N4L`.

**Expect:** an N4L draft cell appears **directly below** the shell cell,
containing the subject, one `"  (cmd) ls -l /` line, and three `"  (out) ` lines
matching what you selected.

## TC-12 A promoted fact parses and reaches the graph

Push the draft cell from TC-11 to the file.

**Expect:** no parse error. Then Ctrl+G to rebuild the graph and search for the
subject.

**Expect:** the subject node exists, with the command and output lines hanging
off it.

## TC-13 Parentheses in output survive as brackets

Run `python3 -c "raise ValueError('boom')"` (or `echo "at foo(bar) line 3"`).
Select the output, give it a subject, promote it, and push to file.

**Expect:** in the draft cell, `(` and `)` appear as `[` and `]`. The push
parses cleanly. *(Unsanitised parentheses are read as arrow syntax and silently
eaten — see `media-fact.js`.)*

## TC-14 Selection edge cases

1. Click once in the output pane so nothing is selected.
   **Expect:** the promote bar disappears.
2. Start a selection inside the output pane and drag well outside it.
   **Expect:** only lines inside the pane are counted and promoted.
3. Select a single line.
   **Expect:** the bar reads `1 line`, singular.

## TC-15 A large run does not hang the app

Run `find / -xdev 2>/dev/null | head -200000`.

**Expect:** the app stays responsive, output scrolls, and the oldest lines are
dropped once the cap is reached. No freeze, no crash.

## TC-16 No shell found

Hard to reproduce on a normal machine; verify by reading rather than running if
necessary.

**Expect:** with no shell discoverable, the cell shows
`No shell found on this system.` rather than an empty dropdown.
```

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-07-30-shell-cell-manual-test-plan.md
git commit -m "docs(test-plan): shell cell"
```

---

## Spec coverage

| Spec requirement | Task |
|---|---|
| `Discover` with an injected `Env` seam; Linux and Windows candidate lists; WSL distros as separate entries | 1 |
| `Shell`/`Kind` types, stable `ID` for persistence | 1 |
| Invocation args per shell family (POSIX / PowerShell / cmd / WSL) | 2 |
| `Run` streaming, stdout/stderr tagging, exit codes, `cwd` validation | 2 |
| stdin closed; `-NonInteractive` | 2 |
| Cancel kills the process group, not just the child | 3 |
| ~50ms batching so a large run cannot flood the bridge | 4 (+ ticker in 5) |
| `ListShells` / `StartShellRun` / `CancelShellRun` + the three events, mirroring the LLM turn API | 5 |
| No timeout | 5 |
| App quit cancels live runs | 5 |
| `buildShellFact` shape: subject, `(cmd)`, one `(out)` per line | 6 |
| `sanitizeTarget`: parens neutralised, leading markers safe by prefix, caps | 6 |
| `cmd`/`out` arrows declared; `ran` rejected for colliding with `runs` | 7 |
| Parser round-trip proof for promoted targets | 7 |
| Scrollback never persisted; ring buffer caps it | 8 |
| `runShell` mirrors `streamTurn`, including the cancel-before-resolve race | 8 |
| `ShellCell.svelte`; dropdown, cwd, command, Run/Stop, output pane | 9 |
| Registrations: `notebook.js`, `cell-label.js`, `cell-preview.js`, `+ Shell` button | 9 |
| Non-zero exit rendered in the pane, never a toast | 9 |
| Read-only renders no Run button | 9 |
| Run disabled while running; `onDestroy` cancels | 9 |
| Selection → subject → `To N4L` → draft cell via `onInsert` | 10 |
| Manual test plan document | 11 |

Nothing in the spec's out-of-scope list is built: no PTY or interactive support,
no persistent session state between runs, no ssh/remote targets, no
environment-variable editor, no command history, no persisted scrollback, no
running on notebook load, and no redaction or secret-scanning of promoted
output.
