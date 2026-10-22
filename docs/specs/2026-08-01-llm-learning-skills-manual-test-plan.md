# LLM learning skills — manual test plan

**Design:** `2026-08-01-llm-learning-skills-design.md`
**Build:** `make dev`

**This document is the verification gate, not a supplement.** Every task in
this feature's plan (1 through 6) was implemented by a headless agent — none
of them could run `make dev` or open a Wails window, and this repository has
no Svelte component test harness. Every picker, warning banner, disabled
button, and settings-tab row described below was reasoned about from source
code and, where possible, unit-tested only in the pieces that could be pulled
out into a plain JS module. None of the actual rendered UI has ever been
watched running. That makes the cases below the first real observation of
this feature, not a regression check against something previously confirmed —
treat a failure here as a new finding, not a surprise.

Automated coverage: `internal/skills` (frontmatter parsing, listing, shadowing,
Duplicate, and the unreadable-file skip) via `make test`; `llm-prompt.js`
(prompt composition and the legacy `mode` migration) and `cell-label.js` via
`make test-js`. Everything below is what those cannot reach: the two Svelte
components themselves, `LLMCell.svelte` and `LLMSettings.svelte`.

Skills live in `<UserConfigDir>/texere/skills/` — on Linux
`~/.config/texere/skills/`.

## Setup notes for two cases below

Two cases below (TC-5, native to the brief, and TC-19) need a notebook whose
LLM cell predates this feature — `mode: "tutor"`, no `skill`/`skillName`. The
app itself never writes that shape anymore, so it has to be hand-crafted in
SQLite; see TC-19 for the exact recipe. TC-17 needs `ListSkills()` to actually
fail rather than just find nothing — see TC-17 for how.

## TC-1 The picker lists both built-ins

Add an LLM cell.

**Expect:** the dropdown offers **Socratic tutor** and **Thinking partner**. The
cell header reads `LLM · Thinking partner`.

## TC-2 A skill frames the conversation

With Thinking partner selected, ask a factual question. Switch to Socratic tutor
and ask it to "just write me a card about X".

**Expect:** the tutor declines to write a finished card and guides you to write
it — the behaviour its prompt specifies. The two skills visibly differ.

## TC-3 The placeholder follows the skill

Switch between the two skills with the input empty.

**Expect:** Thinking partner shows `Ask…`; Socratic tutor shows
`Explain your understanding, or draft a card…`.

## TC-4 The choice persists

Select Socratic tutor, close the notebook, reopen it.

**Expect:** still on Socratic tutor, and the header still reads
`LLM · Socratic tutor`.

## TC-5 A notebook saved before skills existed still works

Open a notebook whose LLM cell predates this change (its saved JSON has
`mode: "tutor"` and no `skill`).

**Expect:** it opens on Socratic tutor, sends normally, and the header labels it.
No error, and the notebook file is NOT rewritten just by opening it.

## TC-6 Your own skill appears

Create `~/.config/texere/skills/exam-drill.md`:

```
---
name: Exam drill
description: rapid recall, no hints
placeholder: Ready?
---
You are an exam drill instructor. Ask short recall questions one at a time. Never give the answer until the user has attempted it.
```

Reopen the LLM cell's picker.

**Expect:** Exam drill is offered, its placeholder is `Ready?`, and selecting it
changes how the model responds.

## TC-7 Duplicating a built-in

⚙ Models → Skills → Duplicate on Socratic tutor.

**Expect:** a file appears at `~/.config/texere/skills/socratic-tutor.md`
containing the full prompt WITH its frontmatter. The row now reads
`yours, overrides built-in`, and its **Duplicate** button is gone. This
matters beyond tidiness: the button's disappearance is the only thing that
makes a second Duplicate on this skill impossible through the UI — without it
there would be nothing stopping a second click from being attempted, only the
backend's refusal to fall back on.

Edit that file — change one sentence — and reopen the tab.

**Expect:** your edit is what the cell now uses.

## TC-8 Deleting your copy restores the built-in

Delete `~/.config/texere/skills/socratic-tutor.md`, reopen the Skills tab.

**Expect:** the row reads `built-in` again and the original prompt is back.

## TC-9 A missing skill is not silently substituted

Select Exam drill in a cell, close the notebook, delete `exam-drill.md`, reopen.

**Expect:** a notice naming the missing skill, the picker NOT switched to
something else, and Send disabled. Choosing a skill clears the notice and
persists normally.

Confirm the saved id was not overwritten: before choosing, close and reopen —
the same notice appears, not a silent fallback.

## TC-10 Read-only wording

Open a notebook read-only whose cell references a deleted skill.

**Expect:** the notice states the fact without telling you to choose one, since
there is nothing to choose with. The picker is disabled.

## TC-11 A malformed skill is visible, not swallowed

Create `broken.md` containing only `---` and a name line, with no closing
delimiter.

**Expect:** it still appears in the list. Nothing vanishes silently.

Create `blank.md` with valid frontmatter and no body.

**Expect:** it appears marked `(no content)`.

## TC-12 Open skills folder

⚙ Models → Skills → **Open skills folder**, with the directory not yet existing.

**Expect:** the folder is created and opens in your file manager.

## TC-13 Switching skills mid-conversation

Send two messages under Thinking partner, then switch to Socratic tutor and send
another.

**Expect:** the transcript is preserved, and the new reply follows the tutor's
method. Switching does not clear the conversation.

## TC-14 A long skill still works

Create a skill whose body is several hundred words.

**Expect:** it is used in full; no truncation and no error.

## TC-15 The Skills tab lists everything, not just what you touch

⚙ Models → Skills, before duplicating anything (a fresh install, or right after
TC-8's cleanup).

**Expect:** two rows, **Socratic tutor** and **Thinking partner**, each showing
its description text and the origin label `built-in`, each with a Duplicate
button, neither marked `(no content)`.

With `exam-drill.md` from TC-6 still present, reopen the tab.

**Expect:** a third row, **Exam drill**, labelled `yours` (not `built-in`, no
Duplicate button), showing its description.

## TC-16 Send is blocked for the instant skills are still loading

`ListSkills()` normally resolves in a few milliseconds — too fast to reliably
catch by hand. Slow it down first by creating enough throwaway skill files that
reading and parsing them takes a moment:

```bash
mkdir -p ~/.config/texere/skills
for i in $(seq 1 1000); do
  printf -- '---\nname: dummy %d\n---\nbody text\n' "$i" > ~/.config/texere/skills/dummy-$i.md
done
```

Open a notebook, add a new LLM cell, and try to click Send (or press
Ctrl/Cmd+Enter) as fast as you can the instant the cell appears.

**Expect:** Send stays disabled until the skill list finishes loading and a
skill resolves — no turn goes out during the window before that.

This is inherently timing-sensitive: on a fast machine with a warm disk cache
the window may be too short to catch even with 1000 dummy files. If you
cannot observe the disabled instant, record this case as **not verified**,
not as passing — do not substitute source-level reasoning for an observation.
To make the window wider and more reliably observable, test on a cold start
(quit the app fully, drop OS disk caches if you can, or simply be the first
one to open an LLM cell after a fresh boot) rather than reopening a cell in an
already-warm session, since `ListSkills()` has furthest to travel the first
time it runs.

Clean up afterward, or later cases will be slower than necessary:

```bash
rm ~/.config/texere/skills/dummy-*.md
```

## TC-17 A failed skills load disables Send and says why

`ListSkills()` only returns an error when the user skills directory exists but
cannot be read as a directory — a missing directory is normal and not an
error. Force the failure by replacing the directory with a plain file of the
same name:

```bash
rm -rf ~/.config/texere/skills
touch ~/.config/texere/skills
```

Open (or reopen) an LLM cell.

**Expect:** the transcript pane shows `Could not load skills: …`, and Send is
disabled — no skill resolves, so no turn can go out.

Restore afterward, or every later case that needs a real skills directory will
also fail:

```bash
rm ~/.config/texere/skills
```

## TC-18 A skill with no body cannot be sent

Create `~/.config/texere/skills/blank.md`:

```
---
name: Blank
---
```

(frontmatter only, no body). Select Blank in an LLM cell's picker.

**Expect:** Send is disabled, and a notice appears — distinct from the
missing-skill notice — reading: `Skill "Blank" has no content yet. Edit its
file to add a system prompt, or choose a different skill below.` No turn can
be sent while Blank is selected.

Open the same notebook read-only.

**Expect:** the notice instead reads `Skill "Blank" has no content, so it
cannot frame a conversation.` — states the fact, does not tell you to edit or
choose, since there is nothing to choose from in read-only.

## TC-19 Opening a legacy notebook does not write to it

Produce a legacy notebook by hand — the app no longer writes a bare `mode`
cell itself, so this has to be edited directly in SQLite:

1. In the app, create a notebook and add an LLM cell; wait a couple of seconds
   for the autosave to settle, then close the app.
2. Find the collection's `learning.db` (`<collectionDir>/learning.db`) and the
   row: `sqlite3 <collectionDir>/learning.db "SELECT notebook_id, cells_json
   FROM notebooks;"`.
3. Edit the LLM cell object in that JSON: remove `"skill"` and `"skillName"`,
   add `"mode":"tutor"`. Write it back:
   `sqlite3 <collectionDir>/learning.db "UPDATE notebooks SET cells_json =
   '<edited JSON>' WHERE notebook_id = '<id>';"`.

Record the file's mtime, open the notebook, and wait at least 2 seconds
without touching anything — no typing, no clicking:

```bash
stat -c '%Y' <collectionDir>/learning.db   # before opening
# open the notebook in the app, wait 2s, do nothing
stat -c '%Y' <collectionDir>/learning.db   # after
```

**Expect:** the cell opens on Socratic tutor and can send normally (as TC-5
describes), and the second `stat` reports the SAME mtime as the first — merely
opening the notebook must not write it. Only sending a message or picking a
different skill should change it.

## TC-20 A read-only cell with a valid skill: shown, disabled picker, no composer

Open, read-only, a notebook whose LLM cell has an ordinary (present,
non-empty) skill selected — not the missing or empty-body cases above.

**Expect:** the cell still names its skill (as TC-4 describes for an editable
notebook), the skill dropdown is visible but disabled, and there is no
composer or Send button at all — read-only notebooks render no composer, so
there is nothing to click, not merely something disabled.

## TC-21 The header updates live when you switch skills, no reopen required

TC-3 checks the placeholder changes when you switch skills; this checks the
cell's own header label, in the same session, without closing or reopening
anything. Add an LLM cell (header reads `LLM · Thinking partner`), then switch
its picker to Socratic tutor.

**Expect:** the header changes to `LLM · Socratic tutor` immediately, in the
same view, with no reopen. This is deliberately distinct from TC-1 (fresh
cell) and TC-4 (persistence across a reopen) — both of those would still pass
even if `selectSkill` persisted the new skill correctly but the header simply
failed to re-render until the notebook was closed and reopened. Only this
case catches that specific defect.
