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

	// Check for survivors BEFORE waiting on `done`, not after. Run only
	// returns once its output pump sees EOF on the child's stdout pipe, and
	// that pipe's write end stays open for as long as ANY holder of it is
	// alive — including an orphaned grandchild that killGroup failed to
	// reach. cmd.Wait()'s WaitDelay-driven pipe-close (the safety valve for
	// exactly that case) only runs once the pump has already returned, so it
	// can never fire in time to rescue this ordering. Concretely: gating this
	// poll behind <-done turns a genuine, unbounded leak into a "Run did not
	// return after cancel" timeout instead of the leak assertion below —
	// confirmed by temporarily reverting killGroup to a direct-child-only
	// kill, which left the leaked pipeline running for its full sleep
	// duration with Run still blocked, not for lack of trying to detect the
	// leak, but because Run itself never returned in that ordering. Polling
	// pgrepCount independently of `done` means the leak assertion is the one
	// that actually trips, regardless of when (or whether) Run returns.
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

	start := time.Now()
	if _, err := Run(ctx, sh, "", "sleep 30", func(Chunk) {}); err == nil {
		t.Log("cancel surfaced as a non-zero exit rather than an error; acceptable")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Run took %v to return after cancel", elapsed)
	}
}
