package shell

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
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

// A copy-buffer boundary can land in the middle of a multi-byte character. If
// the writer emitted each half as its own chunk, both would be invalid UTF-8 and
// the JSON bridge would render one arrow as three replacement characters.
func TestStreamWriterRejoinsARuneSplitAcrossWrites(t *testing.T) {
	var mu sync.Mutex
	var got []Chunk
	w := &streamWriter{mu: &mu, stream: "stdout", sink: func(c Chunk) { got = append(got, c) }}

	arrow := []byte("→") // 3 bytes: e2 86 92
	if _, err := w.Write(append([]byte("ab"), arrow[:1]...)); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "ab" {
		t.Fatalf("first write should emit only the complete prefix, got %+v", got)
	}
	if _, err := w.Write(append(arrow[1:], []byte("cd")...)); err != nil {
		t.Fatal(err)
	}
	w.flush()

	var joined strings.Builder
	for _, c := range got {
		if !utf8.ValidString(c.Text) {
			t.Fatalf("chunk %q is not valid UTF-8", c.Text)
		}
		joined.WriteString(c.Text)
	}
	if joined.String() != "ab→cd" {
		t.Fatalf("got %q, want %q", joined.String(), "ab→cd")
	}
}

// Holding bytes back must never lose them: a stream that ends mid-rune has to
// surface the fragment rather than swallow it.
func TestStreamWriterFlushesATruncatedFinalRune(t *testing.T) {
	var mu sync.Mutex
	var got []Chunk
	w := &streamWriter{mu: &mu, stream: "stderr", sink: func(c Chunk) { got = append(got, c) }}

	if _, err := w.Write([]byte("x\xe2\x86")); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "x" {
		t.Fatalf("got %+v, want only the complete prefix", got)
	}
	w.flush()
	var joined strings.Builder
	for _, c := range got {
		joined.WriteString(c.Text)
	}
	if joined.String() != "x\xe2\x86" {
		t.Fatalf("truncated tail was not flushed: got %q", joined.String())
	}
}

// Bytes that are invalid rather than incomplete must pass straight through, or
// a stray 0xff would stall the stream forever waiting for a rune that is never
// coming.
func TestStreamWriterDoesNotHoldBackGenuineGarbage(t *testing.T) {
	var mu sync.Mutex
	var got []Chunk
	w := &streamWriter{mu: &mu, stream: "stdout", sink: func(c Chunk) { got = append(got, c) }}
	if _, err := w.Write([]byte{'a', 0xff}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "a\xff" {
		t.Fatalf("got %+v, want the invalid byte emitted immediately", got)
	}
}

// End to end: output larger than any one copy buffer, with a multi-byte
// character straddling the boundary, must reach the sink intact.
func TestRunKeepsMultiByteCharactersIntactAcrossChunks(t *testing.T) {
	sh := shTest(t)
	// EVERY chunk must be valid on its own — concatenating the halves of a
	// split rune would hide the bug, and it is the individual chunk that
	// crosses the JSON bridge.
	var mu sync.Mutex
	var all strings.Builder
	sink := func(ch Chunk) {
		mu.Lock()
		defer mu.Unlock()
		if !utf8.ValidString(ch.Text) {
			t.Errorf("chunk of %d bytes is not valid UTF-8", len(ch.Text))
		}
		all.WriteString(ch.Text)
	}

	// 8190 filler bytes puts the arrow astride the 8192-byte mark, and the
	// total exceeds exec's 32KiB copy buffer too.
	cmd := `printf 'x%.0s' $(seq 1 8190); printf '\342\206\222'; printf 'y%.0s' $(seq 1 40000)`
	if _, err := Run(context.Background(), sh, "", cmd, sink); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := all.String()
	if strings.Count(out, "→") != 1 {
		t.Fatalf("got %d arrows, want 1", strings.Count(out, "→"))
	}
	if strings.ContainsRune(out, utf8.RuneError) {
		t.Fatal("output contains a replacement character")
	}
}
