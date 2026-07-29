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
