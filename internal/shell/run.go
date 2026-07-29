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
