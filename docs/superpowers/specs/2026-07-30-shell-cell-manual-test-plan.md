# Shell cell — manual test plan

**Design:** `2026-07-30-shell-cell-design.md`
**Build:** `make dev`

Automated coverage: `internal/shell` (discovery, invocation args, streaming,
exit codes, cwd validation, stdin closed, process-group kill, batching) via
`make test`; `shell-fact.js` and `shell-lines.js` via `make test-js`; promoted
target shapes against the real N4L parser in
`internal/content/shell_target_test.go`. Everything below is what those cannot
reach.

## Why this document is the primary verification gate

Every implementer on this feature worked headless — there is no Svelte
component test harness in this repo, and no one who wrote `ShellCell.svelte`
ever ran `make dev` and clicked the button. The static reasoning in the task
reports (reading the code and arguing about what it must do) is not the same
as having watched it happen. That means the UI behaviour below — dropdown
population, output streaming and tinting, Stop actually killing a process
group, persistence across reopen, read-only gating, selection-to-promote,
and the missing-shell notice — has **never been observed running**, in any
form, by anyone or anything, until a human works through this document.

This is not a supplement to an automated suite that already covers the
feature; for the component-level behaviour it *is* the coverage. What the
automated suites verify instead is the layer below the UI:

- **`internal/shell`, via `make test`** — shell discovery, the exact argv
  built per shell, output streaming/batching, exit-code capture, working-
  directory validation, stdin being closed, and process-group kill on
  cancel. This is real coverage of the OS-facing logic, but it never touches
  a rendered cell, a dropdown, a click, or a text selection.
- **`shell-fact.js` and `shell-lines.js`, via `make test-js`** — the pure
  functions that build an N4L fact string from a subject/command/selected
  lines, and that append streamed chunks into line records. Also real
  coverage, also never rendered.
- **A Go test proving a promoted fact survives the real N4L parser** — that
  a `shell-fact.js`-shaped block parses without error and produces the
  expected targets, exercised end-to-end through the actual parser rather
  than assumed.

None of that reaches: what the dropdown actually shows on a real machine,
whether tinting is visually distinguishable, whether Stop actually leaves no
process behind, whether the promote bar behaves correctly through a real
mouse selection, or whether the missing-shell notice reads correctly and
blocks Run. Every case below exists because something in that gap needed a
human.

Prereqs: a collection with a processing notebook that can be edited, and the
ability to reopen one read-only.

## TC-1 The dropdown lists real shells

Add a `+ Shell` cell.

**Expect:** the dropdown lists shells actually installed on this machine — on
Linux your `$SHELL` first, then bash/zsh/sh without duplicates. The cell header
reads `Shell · <label>`. No entry appears twice.

## TC-2 A command runs and streams

Run `echo hello`.

**Expect:** `hello` in the output pane, then `exit 0 · 0.0s` in muted text.

Then run something slow and chatty — `for i in $(seq 1 50); do echo $i; sleep 0.05; done`.

**Expect:** lines appear progressively, not all at the end.

## TC-3 stderr is distinguished but stays in place

Run `echo out; echo err 1>&2; echo out2`.

**Expect:** all three lines present; the `err` line is tinted differently. Its
position relative to the stdout lines may vary between runs — that is expected
and documented, not a bug.

## TC-4 A non-zero exit is information, not an error

Run `exit 3`. Then run `grep nonexistent /etc/hostname`.

**Expect:** `exit 3` and `exit 1` shown in the output pane, tinted as bad.
**No error toast appears.** The cell is not in an error state.

## TC-5 Stop leaves nothing behind

Run `sleep 300 | cat`. Click **Stop**.

**Expect:** the run ends promptly and the status reads `cancelled`.

Then, in a separate terminal: `ps -ef | grep -E 'sleep 300' | grep -v grep`.

**Expect:** no output. Nothing survived.

*(This is the case `internal/shell/cancel_test.go` covers in Go; repeat it here
because the Windows path uses `taskkill` and has no automated test.)*

## TC-6 stdin is closed, so prompts fail rather than hang

Run `cat`.

**Expect:** returns immediately with `exit 0` — it read EOF, it did not hang.

Run `sudo -k true` (on a machine where sudo needs a password).

**Expect:** it fails with a readable error within a second or two. It does
**not** hang the cell.

## TC-7 Working directory

Set the working directory to `/tmp` and run `pwd`.

**Expect:** `/tmp`.

Set it to `/definitely/not/here` and run `echo hi`.

**Expect:** a readable error naming the directory, and **no output** — it failed
before spawning.

Reopen the notebook.

**Expect:** the working directory is still set.

## TC-8 What persists and what does not

Set a shell, a working directory and a command; run it so there is output. Close
the notebook and reopen it.

**Expect:** shell, working directory, command and subject are all restored. The
**output pane is empty** — scrollback is deliberately never persisted.

## TC-9 Read-only

Open a notebook containing a shell cell read-only.

**Expect:** no dropdown, no working-directory field, **no Run button**, no
promote bar. The command textarea is disabled. The command text is still
readable.

## TC-10 Folding does not interrupt a run

Run `sleep 10; echo done`. While it is running, fold the cell from its header.
Wait, then unfold.

**Expect:** still running when unfolded, and `done` arrives. Folding a shell
cell does not kill its run.

*(Folded cells stay mounted by design — see the 2026-07-29 folding design.)*

## TC-11 Promote a selection

Run `ls -l /`. Select three output lines with the mouse.

**Expect:** the promote bar appears, reading `3 lines`. `To N4L` is disabled
until a subject is typed.

Type a subject and click `To N4L`.

**Expect:** an N4L draft cell appears **directly below** the shell cell,
containing the subject, one `"  (cmd) ls -l /` line, and three `"  (out) ` lines
matching what you selected.

## TC-12 A promoted fact parses and reaches the graph

Push the draft cell from TC-11 to the file.

**Expect:** no parse error. Then Ctrl+G to rebuild the graph and search for the
subject.

**Expect:** the subject node exists, with the command and output lines hanging
off it.

## TC-13 Parentheses in output survive as brackets

Run `python3 -c "raise ValueError('boom')"` (or `echo "at foo(bar) line 3"`).
Select the output, give it a subject, promote it, and push to file.

**Expect:** in the draft cell, `(` and `)` appear as `[` and `]`. The push
parses cleanly. *(Unsanitised parentheses are read as arrow syntax and silently
eaten — see `media-fact.js`.)*

## TC-14 Selection edge cases

1. Click once in the output pane so nothing is selected.
   **Expect:** the promote bar disappears.
2. Start a selection inside the output pane and drag well outside it.
   **Expect:** only lines inside the pane are counted and promoted.
3. Select a single line.
   **Expect:** the bar reads `1 line`, singular.

## TC-15 A large run does not hang the app

Run `find / -xdev 2>/dev/null | head -200000`.

**Expect:** the app stays responsive, output scrolls, and the oldest lines are
dropped once the cap is reached. No freeze, no crash.

## TC-16 No shell found

Hard to reproduce on a normal machine; verify by reading rather than running if
necessary.

**Expect:** with no shell discoverable, the cell shows
`No shell found on this system.` rather than an empty dropdown.

## TC-17 A saved shell that has gone missing — editable notebook

This covers a defect found and fixed during review: the cell used to silently
substitute whatever shell sorted first in place of a saved shell that is no
longer installed, with no visible sign to the user that their choice had been
overridden.

Set up: save a shell cell with a shell that will not exist on the reopening
machine (e.g. edit the saved notebook's JSON to set `shell`/`shellLabel` to
something like `wsl:Ubuntu` or another shell absent here, or move the
notebook to a machine lacking the saved shell). Reopen the notebook, editable
(not read-only).

**Expect:**
1. An amber notice appears above the shell/cwd/Run bar reading exactly:
   `Saved shell "<label-or-id>" was not found on this machine. Choose one below to continue.`
2. The dropdown is visible and populated with real shells, but shows nothing
   selected — the saved value is left untouched, not defaulted to the first
   option.
3. The Run button is present but **disabled**, even with a command typed.
4. The command and working-directory fields are still editable while the
   notice is up.
5. Picking any shell from the dropdown makes the notice disappear immediately
   and enables Run (given a command is present).
6. Close the notebook **without** picking a shell, then reopen it again.
   **Expect:** the same notice reappears, naming the same original shell — it
   must not have silently adopted a different shell in the interim.

## TC-18 A saved shell that has gone missing — read-only notebook

Same setup as TC-17 (a shell cell saved with a shell absent from this
machine), but open the notebook **read-only**.

**Expect:** the amber notice reads exactly:
`Saved shell "<label-or-id>" is not installed on this machine.`

No "Choose one below" or "to continue" wording — read-only has no dropdown to
choose from and nothing to continue toward. Beneath the notice: no dropdown,
no working-directory field, no Run button (all hidden by the same read-only
guard as TC-9). The command text is still readable.

## TC-19 No promote bar in read-only, even with a selection

Open a notebook with a shell cell read-only, where the cell has prior output
visible (or reopen after a run — noting TC-8's rule that output does not
persist, so this may require temporarily testing in an editable notebook
switched to read-only mid-session if the app supports that, or verifying
immediately after a run before closing).

Attempt to select text in the output pane with the mouse.

**Expect:** no promote bar appears, regardless of whether a text selection is
possible or takes visual effect. This is stronger than TC-9's general
"no promote bar" — it specifically confirms the bar stays suppressed even
when the user actively tries to select output, not merely that it's absent
when no one has tried.

## TC-20 Editing the command after selecting output clears the stale selection

This covers a defect found and fixed during review: promoting a selection
left over from a previous run would pair a newly edited command with output
that command never produced — a fact that reads as correct but is wrong.

Run a command, e.g. `ls -l /`. Select some output lines (the promote bar
appears). **Without promoting**, edit the command text in the textarea.
Click **Run** again.

**Expect:** once the new run starts, the promote bar is gone — no stale line
count or stale text is shown. A selection from before a re-run must not
survive into the new run.

## TC-21 Promoting twice does not insert a duplicate

This covers a defect found and fixed during review: nothing previously
stopped a second click on `To N4L` from inserting a second, duplicate draft
cell for the same selection.

Run a command, select output, type a subject, click `To N4L` once.

**Expect:** the draft cell is inserted directly below the shell cell, and the
promote bar disappears immediately afterward. Attempt to click `To N4L`
again (if it is still reachable at all).

**Expect:** no second draft cell is inserted — the bar's disappearance is
what prevents the double-click, since there is no longer a selection to
promote. Note: the **subject field is not cleared** by a successful
promote — this is deliberate, so a user promoting several selections from
the same run doesn't have to retype the subject each time. Confirm the
subject text is still present after promoting.
