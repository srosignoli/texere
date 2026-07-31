// Package shell runs a single command in an OS shell and streams its output.
//
// It is deliberately NOT a terminal: there is no PTY and stdin is closed, so
// anything that prompts (sudo without NOPASSWD, ssh, vim, top) fails fast with
// a readable error rather than hanging forever with no way to answer. That is
// the accepted cost of not carrying a PTY dependency; see the shell-cell design
// doc for the reasoning.
package shell

// Kind selects how a command is handed to a shell. Every shell in one Kind
// takes the same argument shape (see ArgsFor).
type Kind string

const (
	KindPosix      Kind = "posix"
	KindPowerShell Kind = "powershell"
	KindCmd        Kind = "cmd"
	KindWSL        Kind = "wsl"
)

// Shell is one entry in the cell's dropdown.
//
// ID is the stable key persisted on the cell — it must not change between
// releases or a saved notebook loses its shell selection. Label is display-only.
// Distro is set for KindWSL alone, and is what `wsl -d` needs.
type Shell struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Kind   Kind   `json:"kind"`
	Distro string `json:"distro,omitempty"`
}
