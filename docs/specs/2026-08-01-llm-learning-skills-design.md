# LLM learning skills

**Date:** 2026-08-01
**Status:** Approved design
**Feature area:** Processing Notebook — LLM cell

## Summary

Replace the LLM cell's two hardcoded modes with learning **skills**: Markdown
files a user writes, holding the system prompt that frames the conversation.
The two current modes ship as embedded defaults which a user file can shadow by
id. Skills live in the app config directory and are edited in the user's own
editor.

A skill is a system prompt, not an agent — no tools, no autonomy, no loop.

## Motivation

`TUTOR_SYSTEM` in `frontend/src/lib/llm.js` encodes the **Feynman Technique**
and **Wozniak's Rules of Knowledge Formulation**. That is pedagogical domain
knowledge sitting in a JavaScript string literal, where no learner can reach it.

For an application whose thesis is that your notes are plain text you own, the
learning method being the one thing you cannot edit is an inconsistency worth
removing. A user who prefers cloze-drilling to Socratic questioning, or who
studies law rather than languages, currently has no way to say so.

## Grounding in the current implementation

Surveyed before designing:

- **The hardcoding is one line.** `llm.js:33`:
  `const parts = [mode === 'tutor' ? TUTOR_SYSTEM : ASK_SYSTEM]`. Everything
  downstream — the context block, attachments, streaming, cancellation — is
  independent of which prompt was chosen.
- **The surrounding surface is small:** two toolbar buttons
  (`LLMCell.svelte:107-108`), one mode-dependent placeholder (`:138`),
  `newLLMCell('ask')` in `notebook.js:18`, and `cell-label.js:42`'s
  `detail = cell.mode`.
- **`go:embed` is already used** in `main.go` for the frontend bundle, so
  shipping embedded defaults needs no new mechanism.
- **App config already lives at `os.UserConfigDir()/texere/config.json`**
  (`app.go:21`), which is where the LLM and STT profiles are stored.
- **`LLMSettings.svelte` already has a tab system** — `let tab = 'llm'`,
  `setTab()`, `apiFor(tab)` — currently serving LLM and STT profiles. A third
  tab fits the existing structure rather than requiring new chrome.
- **`runtime.BrowserOpenURL` is already used** (`reading_methods.go:352`). A
  `file://` directory URL opens the OS file manager, so "reveal" needs no new
  dependency.
- **The existing fsnotify watcher is scoped to the collection's `.n4l` files**
  (`internal/content/watcher.go`) and is not reusable for a config directory.
- **`buildSystem` has no tests**, because `llm.js` imports the Wails runtime and
  so cannot be loaded by Node's test runner — the same structural trap fixed for
  `shell-lines.js` on the 029 branch.
- **The repo hand-rolls its parsing** (N4L) and carries no YAML dependency.

## Decisions (from brainstorming)

- **Skills live in the app config directory**, `os.UserConfigDir()/texere/skills/`,
  beside the profiles they work with. A learning method is personal — how *you*
  want to be taught — and applies whether the collection is Latin or Kubernetes.
  Rejected: per-collection skills, which would travel with shared notes but
  force you to re-create your preferred method in every collection; and a
  two-tier collection-overrides-config lookup, which buys specialisation at the
  cost of two search paths and collision rules.
- **The two defaults are embedded and always present; a user file shadows them
  by id.** Deleting your copy restores the built-in, so "reset to default" is
  free, and improvements to the shipped prompts reach existing installs unless
  deliberately overridden. Rejected: seeding editable copies on first run, where
  shipped improvements never propagate and there is no way back; and read-only
  built-ins, which force you to retype an entire prompt to change one sentence.
- **Authoring happens outside the app.** Skills are `.md` files edited in the
  user's own editor. Texere adds only `Duplicate` and `Reveal folder`. Rejected:
  an in-app editor, which is a meaningful chunk of UI — file list, dirty state,
  save, validation, delete — for files most people will touch a handful of times.
- **`TRANSCRIBE_SYSTEM` stays hardcoded.** It is a fixed utility for turning
  scanned images into text, not a learning method, and it is invoked by the
  transcribe path rather than by the cell's mode.

## Architecture

### The skill format

```markdown
---
name: Socratic tutor
description: Feynman technique and Wozniak's rules; you write the cards
placeholder: Explain your understanding, or draft a card…
---
You are a Socratic learning tutor. Your goal is to strengthen…
```

**The filename is the id** — `socratic-tutor.md` → `socratic-tutor`. There is no
`id:` field, because two sources of truth drift: renaming the file renames the
skill, which is what a user expects.

| key | required | use |
|---|---|---|
| `name` | no | picker label; falls back to the id |
| `description` | no | picker subtitle |
| `placeholder` | no | the input hint; falls back to `Ask…` |
| *body* | — | everything after the frontmatter: the system prompt |

### `internal/skills/` (new package)

| file | responsibility |
|---|---|
| `skills.go` | the `Skill` type |
| `parse.go` | frontmatter → `Skill`; pure |
| `load.go` | embed + directory listing + shadowing |
| `builtin/*.md` | the two defaults, `go:embed`ed |

```go
type Skill struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Description string `json:"description"`
    Placeholder string `json:"placeholder"`
    Body        string `json:"body"`
    Builtin     bool   `json:"builtin"`
    Overridden  bool   `json:"overridden"`
    Path        string `json:"path"` // user file; "" for a built-in
}

func Parse(id string, data []byte) Skill
func List(dir string) ([]Skill, error)
func Dir() (string, error)
func Duplicate(dir, id string) (string, error)
```

**Frontmatter is read by a small hand-rolled parser, not a YAML dependency.**
Three flat `key: value` strings do not justify a new module, and this repo
hand-rolls its N4L parsing already. The limitation is real and belongs in the
docs: flat keys only, no nesting and no multi-line values.

**`Parse` never returns an error.** A file with no frontmatter becomes a skill
whose entire content is the body and whose name is its id. A skill whose body is
empty is still listed, marked `(no content)` in the settings tab. This is
deliberate: the 029 branch was repeatedly bitten by things that silently
vanished, and a half-edited skill should appear looking wrong rather than
disappear, leaving the user unsure whether the app saw the file at all.

**Shadowing.** `List` reads the embedded built-ins, then the directory; a user
file whose id matches a built-in replaces it and the result carries
`Overridden: true`. Sorting is alphabetical by name regardless of origin, so
order is predictable; origin is shown as a label, not as position.

### `skills_methods.go` (new, beside the other `*_methods.go`)

- `ListSkills() ([]skills.Skill, error)`
- `DuplicateSkill(id string) error` — writes the built-in's body to
  `skills/<id>.md`, creating the directory if needed. **It refuses rather than
  overwrites** when that file already exists: the user file is the one thing
  here Texere did not author, and silently replacing an edited skill with the
  shipped text would destroy work with no undo.
- `RevealSkillsDir() error` — creates the directory if needed, then
  `runtime.BrowserOpenURL("file://" + dir)`

### `frontend/src/lib/llm-prompt.js` (new, dependency-free)

`buildSystem` and the migration mapping move here, out of `llm.js`, which keeps
only `streamTurn`. **This is what finally makes the system-prompt assembly
testable**: `llm.js` imports the Wails runtime, so Node's test runner cannot
load it, and `buildSystem` has consequently never had a test despite composing
the entire prompt.

```js
buildSystem(systemText, context, attachedCells) -> string
skillIdForCell(cell) -> string
```

`buildSystem`'s first parameter changes from a mode to the skill body. It no
longer decides anything — it composes. Its context-block logic is unchanged.

**Migration is lazy and non-destructive.** `skillIdForCell` returns `cell.skill`
when set, otherwise maps the legacy field: `ask` → `thinking-partner`,
`tutor` → `socratic-tutor`. Notebooks saved before this change keep working with
no rewrite pass, and since cells are persisted as one opaque JSON blob, the
stale `mode` field costs nothing.

### `LLMCell.svelte`

The two mode buttons become a dropdown, matching the STT profile picker in
`AudioCell` and the shell picker in `ShellCell`. Persisted fields become
`skill` and `skillName` — the latter exists so `cell-label.js` can render
`LLM · Socratic tutor` without access to the skill list, the same reasoning
behind `shellLabel`. The placeholder comes from the selected skill.

**A missing skill is never silently substituted.** If `cell.skill` is absent
from the list, the cell names it, leaves the saved id untouched, and disables
Send until the user chooses. Wording is conditional on `readOnly`, since a
read-only notebook cannot act on an instruction. This is the rule established
and proven for the shell cell's missing-shell case.

### `LLMSettings.svelte`

A third tab in the existing `apiFor(tab)` structure: the skill list with origin
labels, `[Duplicate]` on built-ins, `[Reveal]` on user files, and one
`Open skills folder` button. Re-listed when the tab opens and after a
Duplicate — no watcher, since this is a folder touched rarely.

### Registrations

- `notebook.js` — `newLLMCell()` defaults to `thinking-partner`
- `cell-label.js` — the `llm` arm reads `cell.skillName` instead of `cell.mode`

## Consequences

- **The system prompt is no longer reviewable in code review.** A user's skill
  can say anything, including something that undermines the learning goal. That
  is the point of the feature, and the same trade every editable-prompt system
  makes.
- **A skill body is sent to the model verbatim on every turn**, so a long skill
  costs tokens on every message. The built-ins are a few hundred words; nothing
  enforces a cap.
- **Skills are not portable with a collection.** Sharing a collection does not
  share the skill its notebooks reference; the recipient sees the missing-skill
  notice. Accepted under the location decision.

## Testing

**Go unit:**

- `Parse`: frontmatter present, absent, partial, with unknown keys, with CRLF
  line endings, with a body containing `---`, and an empty body. Assert it never
  errors and that a missing `name` falls back to the id.
- `List` against a temp dir: built-ins alone; a user skill added; a user file
  shadowing a built-in sets `Overridden`; deleting it restores the built-in;
  a non-`.md` file is ignored; sorting is by name.
- `Duplicate`: writes the built-in body, creates the directory, and refuses to
  clobber an existing user file.

**JS unit (`llm-prompt.test.js`):**

- `buildSystem`: the skill body leads; each context block appears when its data
  is present and is absent when not; each attachment type formats as specified;
  no context yields the body alone.
- `skillIdForCell`: `cell.skill` wins; legacy `ask` and `tutor` map correctly;
  an unknown legacy mode and an empty cell fall back to the default.
- `cell-label.test.js`: an LLM cell renders its `skillName`, and one with
  neither `skillName` nor `mode` renders `LLM` with no dangling separator.

**Manual** — its own test-plan document, matching prior slices: the picker lists
built-ins and user skills with origins; editing a file and reopening the tab
shows the change; Duplicate then edit shadows the built-in and deleting restores
it; a notebook saved with `mode: 'tutor'` opens on the Socratic tutor; a cell
referencing a deleted skill shows the notice, keeps its saved id and cannot
Send; read-only wording omits the call to action; the placeholder follows the
skill; and a conversation started under one skill continues correctly after
switching.

## Out of scope

- Skills selecting a model or LLM profile.
- Skills declaring which context blocks to include.
- Per-collection skills, and any collection/config override chain.
- An in-app skill editor.
- Sharing, importing or downloading skills.
- A file watcher on the skills directory.
- Changing `TRANSCRIBE_SYSTEM` or the transcribe path.
- Any change to streaming, cancellation, attachments or the context block's
  content.
