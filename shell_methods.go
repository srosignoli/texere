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
