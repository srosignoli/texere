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
