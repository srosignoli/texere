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
