# Shell cell

**Date:** 2026-07-30
**Status:** Approved design
**Feature area:** Processing Notebook — new cell type

## Summary

A notebook cell that runs a command in an OS shell and streams the output back.
The shell is chosen from a dropdown listing what is actually installed — the
system shell on Linux, PowerShell and each WSL distro on Windows. Selecting any
part of the output and giving it a subject promotes it into an N4L draft cell as
a fact about the command that produced it.

The target user is a sysadmin or devops engineer whose knowledge *is* command
plus output, and whose notes are runbooks.

## Motivation

The notebook can already capture a picture, a recording, a sketch, a graph query
and a JavaScript computation, and promote each into N4L. It cannot capture the
thing a sysadmin actually produces all day. A shell cell closes that gap, and
the promote step is the point: an entry that records *what was run* and *what it
said* is a runbook line, where either half alone is nearly worthless.

## Grounding in the current implementation

Surveyed before designing:

- **There is no `os/exec` anywhere in the Go codebase.** This is greenfield, not
  a retrofit. No PTY dependency exists either.
- **A streaming pattern is already established.** `StartLLMTurn(req) -> streamID`
  emits `llm:delta` / `llm:done` / `llm:error` keyed by that ID, with
  `CancelLLMTurn(streamID)` to stop it; `frontend/src/lib/llm.js:72-74` filters
  on `streamId` via `EventsOn`. `runtime.EventsEmit` is already used across five
  other Go files. The shell cell mirrors this rather than inventing a mechanism.
- **`compute-host.js` is the closest precedent for a run host:** it returns
  `{cancel}`, streams via callbacks, and enforces a 5s timeout.
- **Notebooks persist as one opaque JSON blob** —
  `SaveNotebook(nbId, JSON.stringify(cells))` on a 600ms debounce — into SQLite
  via `learnStore`, keyed by extract node. Notebooks are local to a collection,
  not imported from third parties.
- **The promote format is `<subject>` then `"  (<code>) <target>`**, per
  `buildMediaFact` in `frontend/src/lib/media-fact.js`. That file carries a long
  comment recording that a parenthesised timestamp is *silently consumed by the
  parser*, because `(...)` is arrow syntax — and that a leading `@` is safe
  **only because the ditto prefix means it is never the first character**.
- **`internal/content/parser.go:219-221`: a line starting with `#` returns
  immediately** — pure comment, never enters the graph. So comments are inert
  and invisible to search.
- **Arrows are declared in pairs** in `SSTconfig/arrows-EP-3.sst`:
  `+ has picture (pic) - is a picture of (pic-of)`. `runs`, `runafter`,
  `runbefore` and `runsin` are already taken; `cmd`, `out`, `outof` and `runfor`
  are free. Commit `582ca30` records a prior reverse-code collision, so codes
  are checked before declaring.
- **Two dropdown precedents exist** for picking a backend resource: `LLMCell`'s
  profile picker and `AudioCell`'s STT-profile picker, both fetching a list on
  mount and persisting the choice on the cell.
- **Folded cells stay mounted** (2026-07-29 folding design), so component-local
  state survives folding.
- **There is no Svelte component test harness.** `frontend/src/lib/*.test.js` run
  under `node --test` via `make test-js`.

## Decisions (from brainstorming)

- **A command runner, not a terminal. No PTY.** Covers `docker ps`,
  `kubectl get`, `systemctl status`, `git log`, pipelines, and long-running
  commands with a Stop button. Excludes `ssh`, `top`, `vim`, sudo password
  prompts and `docker exec -it`. Roughly a quarter the work of a real terminal,
  needs zero new dependencies, and captures most of what is worth putting in a
  note — the interactive cases are the ones least likely to become runbook
  entries. A full PTY (`creack/pty` + ConPTY + `xterm.js`, all pure Go / standard
  JS) remains possible as a later slice.
- **The promoted artifact is a fact about the command**, mirroring `MediaCell`:
  the user types a subject, and the cell builds `(cmd)` for the command plus one
  `(out)` per selected output line. Real graph content, searchable, and the
  command travels with its output. Rejected: output as inert `#` comments —
  verbatim and needing no sanitising, but invisible to the graph, which defeats
  the purpose of promoting it. Also rejected: doing both, as double the promote
  logic for a redundant copy.
- **Explicit Run only; no further ceremony.** Never on notebook load, never in
  read-only, only on an explicit Run. Nothing more. It is the user's own machine
  and shell; a confirmation dialog is friction they would route around by
  opening a terminal. The accepted cost: a notebook synced from another machine
  arrives with a stored command one click from running.
- **The shell is a dropdown, not a baked-in default.** Discovery lists what is
  installed and the user picks; the choice persists on the cell. This replaced
  three proposed default-ordering policies — with a picker, no policy is needed,
  and multiple WSL distros become distinct named entries rather than a vague
  "WSL".
- **Scrollback is never persisted.** Session output is ephemeral scratch; the
  promoted N4L is the durable record. This is what keeps the cell cheap to
  store, unlike the audio and image cells.
- **No timeout.** `ComputeCell`'s 5s cap is right for a JS sandbox and wrong
  here — `docker build` takes minutes. A Stop button replaces it; volume is
  capped instead of time.

## Architecture

### `internal/shell/` (new package)

**`Discover(env) []Shell`** returns `{ID, Label, Path, Args}` for each shell
found. `env` is an injected seam (`LookPath`, `Getenv`, and a WSL-distro lister)
so discovery is unit-testable without the real OS — without it, discovery is
untestable on any single platform, and this repo cross-compiles for Windows.

- Linux: `$SHELL`, then `/bin/bash`, `/bin/zsh`, `/bin/sh`; deduped by resolved
  path.
- Windows: `pwsh.exe`, `powershell.exe`, one entry per distro from
  `wsl.exe -l -q`, then `cmd.exe`.

**`Run(ctx, sh, cwd, command, sink)`** wraps `exec.CommandContext`. Invocation
per shell family:

| family | args |
|---|---|
| POSIX | `-c <command>` |
| PowerShell | `-NoProfile -NonInteractive -Command <command>` |
| cmd | `/C <command>` |
| WSL | `-d <distro> -- bash -lc <command>` |

Three details that carry more weight than they look:

- **stdin is closed, deliberately.** With no PTY, anything that prompts would
  hang forever with no way to answer. Closing stdin makes `sudo` fail
  immediately with a readable error instead of hanging the cell;
  `-NonInteractive` does the same for PowerShell. This is the honest cost of the
  no-PTY choice, and it must fail loudly rather than mysteriously.
- **stdout and stderr stream merged in arrival order, each chunk tagged.** A
  terminal interleaves them and so should this; the tag lets the UI tint stderr
  without reordering anything.
- **Cancel kills the process group, not just the child.**
  `exec.CommandContext` signals only the direct process, so
  `bash -c "sleep 100 | cat"` would leave an orphan. Unix: `Setpgid` and signal
  `-pid`. Windows: a job object, or `taskkill /T`. Without this, every Stop
  leaks processes — unacceptable in a devops tool.

**Chunks are coalesced into ~50ms batches before emission.** 100k separate Wails
events would flood the bridge regardless of memory.

### `shell_methods.go` (new, beside the other `*_methods.go`)

Mirrors the LLM turn API exactly:

- `ListShells() []Shell`
- `StartShellRun(req) (runID string, err error)` — emits `shell:out`
  `{runID, stream, text}`, `shell:exit` `{runID, code, durationMs}`,
  `shell:error` `{runID, message}`
- `CancelShellRun(runID string)`

Live runs are held in a mutex-guarded `map[string]context.CancelFunc`, as the LLM
turn registry does. `app:close-requested` (`app.go:140`) cancels all live runs so
quitting does not orphan processes.

### `frontend/src/lib/shell-run.js` (new)

Wraps `EventsOn` and returns `{cancel}` — the same shape `compute-host.js`
returns, so `ShellCell` handles a run the way `ComputeCell` does. Holds the ring
buffer bound: the last ~5,000 lines, dropping from the front with a truncation
marker, or `yes` exhausts the webview.

### `frontend/src/lib/shell-fact.js` + `.test.js` (new)

Pure, dependency-free, mirroring `media-fact.js`. Exports:

- `buildShellFact({subject, command, lines}) -> string` — `''` when the subject
  is blank or there are no lines, as `buildMediaFact` returns `''` without a
  subject.
- `sanitizeTarget(line) -> string`

**The sanitiser is narrower than it first appears, and this is a real finding.**
Each output line becomes the *target* of a ditto line — `"  (out) <line>` — so it
never sits at the start of a line. That defuses the leading-character hazards
(`#`, `"`, `-`, `::`, `@`) automatically, and it is exactly the argument
`media-fact.js` already makes for `@`. What remains genuinely dangerous is
**parentheses in the target**, since `(...)` is arrow syntax wherever it appears
— the trap that comment documents for timestamps. Plus empty lines, and a
per-line length cap.

**A cross-language testing gap, closed.** The sanitiser is JavaScript and the
parser is Go, so a `node:test` can only assert the sanitiser's output *shape*,
never that Go accepts it. `media-fact.js` handled this with a hand-verified
comment. This design makes it a real test: a Go test in `internal/content` feeds
the documented safe shapes through the actual parser and asserts the target
survives intact. Cheap, because the parser is already test-harnessed.

### `frontend/src/components/notebook/ShellCell.svelte` (new)

```
┌ SHELL · Ubuntu                        ▾  ▲ ▼   ✕ ┐   ← existing header
│ [Ubuntu ▾]  cwd [/var/log        ]        [Run]  │
│ $ docker ps --format '{{.Names}}'                │
├──────────────────────────────────────────────────┤
│ pg-primary                                       │
│ redis-cache                                      │
│ exit 0 · 0.4s                                    │
├──────────────────────────────────────────────────┤
│ subject [check containers on prod ]   [To N4L]   │   ← when a selection exists
└──────────────────────────────────────────────────┘
```

Selection is `window.getSelection()` scoped to the output element — no xterm
means plain DOM, so no addon and no API. **To N4L** calls the existing
`onInsert(buildShellFact(...))`, inserting an N4L draft cell after this one,
exactly as `TranscribeBar` does. The subject-plus-disabled-button pattern is
lifted from `MediaCell`.

### Registrations (four one-liners)

- `notebook.js` — `shell: ShellCell` in `cellRegistry`, plus `newShellCell()`
- `cell-label.js` — `shell: 'Shell'`, detail = the selected shell's label
- `cell-preview.js` — a `shell` arm returning the first line of `cell.command`
- `NotebookPane.svelte` — a `+ Shell` button

### `SSTconfig/arrows-EP-3.sst`

Declared beside the media arrows. Codes verified free against every existing
declaration; `ran` was rejected for sitting confusingly next to the existing
`runs`, which is the collision class recorded in `582ca30`.

```
 + has command  (cmd)  - was run for      (runfor)
 + has output   (out)  - is the output of (outof)
```

## Persistence

| field | persisted | why |
|---|---|---|
| `command` | yes | iterate on a pipeline across sessions |
| `shell` | yes | the dropdown choice |
| `cwd` | yes | the mitigation for having no persistent session |
| `subject` | yes | as `MediaCell` persists its subject |
| scrollback | **no** | see below |

Scrollback stays in component-local state. Keeping it in `cell` would mean
`docker logs` rewriting the entire notebook JSON blob, repeatedly, at megabyte
scale.

That puts `ShellCell` in the same category as `ComputeCell`, whose `logs` and
`chartConfigs` are also component-only — so the 2026-07-29 folding design
protects it for free: folded cells stay mounted, so **folding a shell cell keeps
its output; closing the notebook does not.** Intended, but it makes one rule
load-bearing for the user: promote before you close.

## Lifecycle

Each rule below already has a precedent in a sibling cell; none is new
machinery.

| rule | precedent |
|---|---|
| Run is disabled while a run is in flight | `LLMCell`'s `disabled={streaming}` |
| `onDestroy` cancels the live run | `LLMCell`'s `turn?.cancel()`, `ComputeCell`'s `run?.cancel()` |
| Quitting the app cancels every live run | `app:close-requested` (`app.go:140`) |
| A read-only notebook renders no Run button at all | the exec-posture invariant |

The three exec-posture invariants, stated once so they can be tested directly:
**never run on notebook load; never run when `readOnly`; run only on an explicit
Run.**

## Error handling

A **non-zero exit is information, not a failure** — rendered as `exit 1 · 2.3s`,
tinted, in the output pane. Never an error toast: `grep` returning 1 is normal.

Actual failures surface as cell errors before anything runs: no shell
discovered (the cell says so rather than showing an empty picker), `cwd` does not
exist (checked before spawning), spawn refused. A cancelled run reads
`exit — cancelled`.

## Consequences

- **Each run is a fresh process**, so `cd /var/log` does not carry to the next
  run and neither do exported variables. The persisted `cwd` field is the
  mitigation. If this chafes, a persistent session becomes slice 2 — it needs
  prompt-boundary detection, which is why it is not slice 1.
- **Anything that prompts fails instead of hanging.** Correct, but it means
  `sudo` without `NOPASSWD` simply does not work here.
- **A synced notebook can arrive with a stored command.** Accepted deliberately
  under the exec-posture decision; the invariants (never on load, never in
  read-only, only on explicit Run) are what keep it to one deliberate click.
- **Output containing secrets can be promoted into a `.n4l` file** that may be
  committed or synced. Selection is manual and the subject is typed by hand, so
  promotion is always deliberate — but nothing redacts, and nothing warns.

## Testing

**Go unit:**

- `Discover` against a faked env: Linux with `$SHELL` set and unset; dedup by
  resolved path; Windows with and without WSL; multiple distros; nothing
  available at all.
- Invocation args per shell family (the table above), asserted exactly.
- `Run`: exit code for success and failure; stderr chunks tagged and interleaved
  in arrival order; `cwd` honoured; a missing `cwd` failing before spawn.
- **Cancel kills the process group:** spawn `sh -c 'sleep 30 | cat'`, cancel,
  assert both processes are gone. This is the test that protects against leaking
  a process on every Stop.
- Chunk coalescing: many rapid writes produce batched emissions, not one per
  line.
- **Parser round-trip** (`internal/content`): each documented safe target shape
  from `sanitizeTarget` survives the real parser with the target intact.

**JS unit (`shell-fact.test.js`):**

- `buildShellFact` shape: subject line, one `(cmd)` line, one `(out)` line per
  output line.
- `sanitizeTarget`: parentheses neutralised; empty lines dropped; over-long
  lines capped; and the leading-character cases (`#`, `"`, `-`, `::`, `@`)
  asserted to pass through untouched, documenting *why* they are safe.
- Empty cases: blank subject, no lines, whitespace-only subject → `''`.
- `cell-preview.js`'s `shell` arm and `cell-label.js`'s `shell` entry, added to
  their existing test files.

**Manual** — its own test-plan document, matching every prior slice: the
dropdown lists real shells; a command runs and streams; stderr is distinguished;
Stop actually stops and leaves no orphan (`ps` afterwards); a non-zero exit is
not an error toast; selection plus subject promotes to a draft cell that pushes
to file and parses; read-only shows no Run; folding mid-run keeps the session;
closing the notebook loses the scrollback as designed.

## Out of scope

- PTY and anything interactive — `ssh`, `top`, `vim`, sudo prompts,
  `docker exec -it`.
- Persistent session state between runs (`cd`, exported variables).
- Remote or ssh targets.
- An environment-variable editor.
- Command history.
- Persisting scrollback.
- Running anything on notebook load.
- Redaction or secret-scanning of promoted output.
