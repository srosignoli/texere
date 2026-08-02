// Package skills loads the learning skills that frame an LLM cell's
// conversation: Markdown files whose body is used verbatim as the system
// prompt.
//
// A skill is a PROMPT, not an agent. There are no tools, no loop and no
// autonomy — the body is one string prepended to the context block that the
// notebook assembles. Keeping that boundary is what keeps this package small.
package skills

// Skill is one entry in the LLM cell's picker.
//
// ID is the filename without its extension, and is what a notebook cell
// persists — it must stay stable, or a saved notebook loses its skill. Body is
// the system prompt, used verbatim.
type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Placeholder string `json:"placeholder"`
	Body        string `json:"body"`

	// Builtin marks a skill shipped inside the binary. Overridden marks a
	// builtin that a user file of the same id has replaced; the surviving entry
	// is the user's, and deleting that file restores the builtin.
	Builtin    bool   `json:"builtin"`
	Overridden bool   `json:"overridden"`
	Path       string `json:"path"` // user file; "" for a builtin
}
