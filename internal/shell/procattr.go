package shell

import "os/exec"

// Replaced in Task 3 by per-platform implementations that put the child in its
// own process group and kill the whole group. Until then, cancellation falls
// back to signalling only the direct child, which leaks pipeline members.
func setProcAttr(c *exec.Cmd) {}

func killGroup(c *exec.Cmd) error {
	if c.Process == nil {
		return nil
	}
	return c.Process.Kill()
}
