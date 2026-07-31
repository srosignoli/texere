package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
	"unicode/utf8"
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

// partialRuneTail reports how many bytes at the end of b are the start of a
// UTF-8 rune that has not arrived in full yet. It returns 0 when b ends on a
// rune boundary, and 0 for bytes that are genuinely invalid rather than merely
// incomplete — holding those back forever would swallow them.
func partialRuneTail(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	// A well-formed final rune (including a real U+FFFD, which decodes with
	// size 3) means there is nothing to hold back.
	if r, size := utf8.DecodeLastRune(b); r != utf8.RuneError || size > 1 {
		return 0
	}
	// Otherwise walk back to the last lead byte. A sequence is incomplete —
	// and so worth holding — only if its lead byte promises more bytes than
	// are present. No lead byte within UTFMax means the tail is junk, not a
	// truncated rune.
	for i := 1; i <= utf8.UTFMax && i <= len(b); i++ {
		if utf8.RuneStart(b[len(b)-i]) {
			if utf8.FullRune(b[len(b)-i:]) {
				return 0
			}
			return i
		}
	}
	return 0
}

// streamWriter is what cmd.Stdout / cmd.Stderr are set to. Using a plain
// io.Writer rather than StdoutPipe is deliberate — see the WaitDelay comment in
// Run.
//
// Two of these share one mutex so sink is never called concurrently, and each
// holds back a trailing partial UTF-8 rune. Without that hold-back a multi-byte
// character straddling a copy-buffer boundary is split into two chunks, each
// invalid UTF-8, and the JSON bridge to the frontend turns the pair into
// replacement characters — mojibake that a promote would then write permanently
// into the user's .n4l file.
type streamWriter struct {
	mu      *sync.Mutex
	stream  string
	sink    func(Chunk)
	pending []byte // trailing bytes of an as-yet-incomplete rune
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	data := p
	if len(w.pending) > 0 {
		data = append(w.pending, p...)
		w.pending = nil
	}
	if n := partialRuneTail(data); n > 0 {
		w.pending = append([]byte(nil), data[len(data)-n:]...)
		data = data[:len(data)-n]
	}
	if len(data) > 0 {
		w.sink(Chunk{Stream: w.stream, Text: string(data)})
	}
	// Always report a full write: a short count would abort exec's copy.
	return len(p), nil
}

// flush emits anything still held back. Called once the stream is over, so a
// genuinely truncated final rune surfaces instead of vanishing.
func (w *streamWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		w.sink(Chunk{Stream: w.stream, Text: string(w.pending)})
		w.pending = nil
	}
}

// Run executes command in sh and streams tagged output to sink until the
// process exits or ctx is cancelled.
//
// sink is called from the two copying goroutines exec owns, but is serialised by
// a mutex, so implementations need no locking of their own. Interleaving BETWEEN
// stdout and stderr is best-effort and must not be relied upon; order within one
// stream is preserved.
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

	// Output goes to plain io.Writers, NOT to StdoutPipe/StderrPipe, and that
	// choice is what makes WaitDelay below work.
	//
	// exec only spawns its own copying goroutines — and only records them in the
	// internal goroutine list that WaitDelay's force-close is gated on — when
	// Stdout/Stderr are set to something that is not an *os.File. StdoutPipe
	// hands back the pipe instead and registers no goroutine, so WaitDelay's
	// pipe-closing branch is skipped entirely and the delay protects nothing.
	// (go1.25 os/exec: writerDescriptor appends to both parentIOPipes and
	// goroutine; StdoutPipe appends only to parentIOPipes, leaving goroutineErr
	// nil, which is the condition awaitGoroutines and watchCtx both check.)
	//
	// The hazard being insured against is real: a grandchild that escapes
	// killGroup (`setsid`, a daemonised child) keeps the write end of the pipe
	// open, so the copy never sees EOF. With writers, WaitDelay after
	// cancellation closes the parent's read ends, the copies fail, and Wait
	// returns. Verified by cancelling `sh -c 'exec 3>&1; setsid sleep 3600 >&3'`:
	// with pipes Run never returned at all; with writers it returns in about
	// WaitDelay.
	var mu sync.Mutex
	stdout := &streamWriter{mu: &mu, stream: "stdout", sink: sink}
	stderr := &streamWriter{mu: &mu, stream: "stderr", sink: sink}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 5 * time.Second

	started := time.Now()
	waitErr := cmd.Run()
	// Wait has already joined the copying goroutines, so nothing can still be
	// writing; this only releases a final partial rune, if any.
	stdout.flush()
	stderr.flush()

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
