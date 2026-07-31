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
	// No self-exclusion for ps's own argv here: marker is a nanosecond
	// timestamp (digits only), and "ps -eo args" contains no digits, so it
	// can never collide.
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, marker) {
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

	// The marker must land in a LEAF process's own argv, not just the
	// top-level `sh -c "..."` argv. A trailing shell comment (`# marker`)
	// only ever appears in sh's own argv — ps -eo args on the sleep/cat
	// children shows bare "sleep 30" / "cat", with no trace of it, so that
	// style of marker would report zero survivors even while the pipeline
	// leaked as reparented orphans. (Verified: pointing killGroup at only the
	// direct child still made a comment-based marker assertion pass.) A
	// unique fractional-second sleep duration puts the marker directly in
	// each leaf process's own argv, where ps can actually see it on the
	// process itself. Both pipeline members carry it, so a leak of either
	// half is caught, not just the first.
	marker := strconv.FormatInt(time.Now().UnixNano(), 10)
	command := "sleep 30." + marker + " | sleep 31." + marker

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = Run(ctx, sh, "", command, func(Chunk) {})
	}()

	// Give the shell time to fork the pipeline. Both legs should show up.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && pgrepCount(t, marker) < 2 {
		time.Sleep(20 * time.Millisecond)
	}
	if n := pgrepCount(t, marker); n < 2 {
		cancel()
		t.Fatalf("pipeline never fully started (found %d of 2 tagged processes)", n)
	}

	cancel()

	// Check for survivors BEFORE waiting on `done`, not after. Run returns
	// once cmd.Wait() has reaped the child and joined exec's copying
	// goroutines, and those goroutines only finish when the write end of the
	// child's stdout pipe is closed by every holder — including any process
	// killGroup failed to reach. Run does now bound that wait (cmd.Stdout is a
	// plain io.Writer, which is what arms cmd.WaitDelay's force-close of the
	// pipes; see run.go), so a leak costs at most WaitDelay rather than
	// hanging forever. But bounded is not free: gating this poll behind <-done
	// would delay the leak check by up to WaitDelay and, worse, report a
	// genuine leak as a slow return rather than as the leak it is. Polling
	// pgrepCount independently of `done` keeps the leak assertion below the
	// one that trips, and leaves "did Run come back?" as the separate,
	// secondary check it should be.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && pgrepCount(t, marker) > 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if n := pgrepCount(t, marker); n > 0 {
		t.Fatalf("%d process(es) matching %q survived cancel", n, marker)
	}

	// Now that we know nothing leaked, Run itself should also have returned
	// promptly — a secondary check, not the leak assertion.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestCancelledRunReturnsPromptly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test")
	}
	sh := Shell{ID: "sh", Path: "/bin/sh", Kind: KindPosix}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()

	// Run in a goroutine and bound the wait with a select. Measuring elapsed
	// time only after Run returns would mean that the one failure this test
	// exists to catch — Run never returning at all — kills the package on its
	// timeout instead of reporting "Run took N to return after cancel".
	type outcome struct {
		err     error
		elapsed time.Duration
	}
	res := make(chan outcome, 1)
	start := time.Now()
	go func() {
		_, err := Run(ctx, sh, "", "sleep 30", func(Chunk) {})
		res <- outcome{err: err, elapsed: time.Since(start)}
	}()

	select {
	case got := <-res:
		if got.err == nil {
			t.Log("cancel surfaced as a non-zero exit rather than an error; acceptable")
		}
		if got.elapsed > 5*time.Second {
			t.Fatalf("Run took %v to return after cancel", got.elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return within 10s of cancel")
	}
}
