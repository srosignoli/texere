# Malformed `<…>` no longer kills the app — manual test plan

**Fix:** `32e4caa` — `internal/search/dirac.go`, `internal/search/search.go`,
`frontend/src/lib/query-grammar.js`
**Build:** `make dev`

## What was wrong

Any search token starting `<` and ending `>` is treated as node-path shorthand.
When the inside does not split into 2 or 3 parts on `|`, upstream prints
`Bad Dirac notation` and calls **`os.Exit(-1)`** — taking the **whole
application** down. The window vanishes and **unsaved editor buffers go with
it**. `Search`'s deferred `recover()` cannot help: `os.Exit` is not a panic.

**The trigger was documented behaviour.** `query-grammar.js` described proximity
search as `a <-> b`, which produces the token `<->` — no `|`, so it exits.
Following the help panel's own instructions killed the app.

Two changes: malformed tokens are now refused with an explanation, and the
proximity entry is gone from the grammar, since the query never reaches the
engine that would have honoured it.

Automated coverage: `internal/search/dirac_guard_test.go` — five malformed forms
and two valid ones. These tests are unusual: a missing guard does not fail them,
it **terminates the test binary**, so a crashed run is the failure signal.
Verified by negative control: removing the guard reproduces the exit.

## Prereqs

Open a collection with a built graph. **Type something into the note editor and
leave it unsaved** — the point of TC-1 is that it survives.

## TC-1 The crash is gone, and your unsaved work survives

Type unsaved text in the note editor. Open the omnibar in graph mode
(Ctrl+Shift+K) and search:

```
brain <-> wave
```

**Expect:** the app stays open. An error appears saying the token cannot be used
and naming the valid forms. **Return to the editor and confirm your unsaved text
is still there.**

*(Before this fix the window closed immediately and that text was gone.)*

## TC-2 The other malformed shapes are handled too

Try each:

```
a <2> b
<justone>
<a|b|c|d>
<>
```

**Expect:** an error each time, naming the offending token. No crash, and
ordinary searching still works afterwards.

## TC-3 Valid node-path shorthand still works

Using two node names that exist in your collection:

```
<some node|another node>
```

and with a context:

```
<some node|some context|another node>
```

**Expect:** these run as before — they are shorthand for `\from … \to …`. They
must **not** be refused.

*(If these break, the guard is over-eager and is rejecting what upstream would
have accepted.)*

## TC-4 The help panel no longer promises proximity

Open the query help panel (the `?` button next to the query bar) and read the
"Matching words" section.

**Expect:** no entry for proximity `<->` / `<N>`. The node-path shorthand entry
is still there.

*(It was removed because it never worked through this bar — the query is taken
as node-path shorthand before it can reach the text-matching engine. Leaving it
documented meant the help panel was instructing users to crash the app.)*

## TC-5 Ordinary search is unaffected

Run several normal queries — plain text, `\chapter`, `\context`, `\notes`,
`\from … \to …`, an exact `!word!`, a phrase `"…"`.

**Expect:** all unchanged. The guard only inspects tokens that both start `<`
and end `>`.

## TC-6 A `<…>` inside quotes

```
"a <-> b"
```

**Expect:** record what happens. The guard checks whitespace-delimited tokens, so
a quoted phrase containing `<->` still yields the bare token `<->` after
splitting and should be refused rather than crash. Either way the app must
survive — that is the property under test.

## What I most want to hear about

- **TC-1's unsaved text.** That is the real cost of this bug and the reason it
  was worth stopping the feature work for.
- **Any `<…>` query that still closes the app.** The guard mirrors upstream's own
  condition, but if a shape gets through, it is the same class of data loss.
- Whether you ever used proximity `<->` and want it working. It is achievable —
  the engine supports it — but needs the token routed past the node-path check,
  which is a feature rather than a crash fix.
