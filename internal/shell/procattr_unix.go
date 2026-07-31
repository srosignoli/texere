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
