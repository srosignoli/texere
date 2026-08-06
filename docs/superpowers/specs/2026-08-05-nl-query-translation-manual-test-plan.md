# Natural-language query translation — manual test plan

**Design:** `2026-08-05-nl-query-translation-design.md`
**Build:** `make dev`

**This document is the verification gate, not a supplement.** The visible part of
this feature lives in `OmniBar.svelte`, and this repository has **no Svelte
component test harness** — so the button, the picker and the refusal card have
**zero automated coverage** and have never been watched running. Treat a failure
here as a new finding, not a surprise.

What *is* automated, and heavily: `internal/corpustest` builds a graph from nine
real `sstorytime-kv` example files and asserts **all 19 few-shot examples return
results** — not merely that they parse. `query-prompt.test.js` covers the prompt
builder, `stripToQuery` and `validateQuery` in 13 cases. `search_llm_methods_test.go`
covers accumulation, error surfacing, the 90-second timeout and the profile
round-trip. Everything below is what those cannot reach.

## Two things that are deliberate — do not report them as bugs

- **The translated query runs.** It lands in the bar and the bar searches as you
  type, so results appear about 300ms later. You see the query *and* whether it
  found anything. An earlier draft said it would never auto-run; that was wrong
  about this bar and the spec was corrected.
- **Expensive queries still wait for Enter.** `\notes`, `\story`, `\sequence` and
  `\from` scan the whole database, so they only run when you press Enter —
  exactly as they already did for typed queries.

## Before you start

You need at least one LLM profile configured (Settings → LLM). For TC-1 you need
to *remove* them all temporarily, so note what you have first.

A built graph helps: the live chapter and context names come from it.

## TC-1 With no LLM profile, the feature is invisible

Remove every LLM profile. Open the omnibar in graph mode (Ctrl+Shift+K).

**Expect:** no ✨ button, no model picker. Graph search behaves exactly as before.

*(Not greyed out, not a tooltip — absent. A user who has never configured a
provider should not see a control they cannot use.)*

Restore your profiles before continuing.

## TC-2 The button appears in graph mode only

With a profile configured, open the omnibar in fuzzy mode, then switch to graph
mode.

**Expect:** no ✨ in fuzzy mode; ✨ and the picker in graph mode.

## TC-3 A question becomes a query, in a separate box

In graph mode you should see **two inputs**: an ask box on top and the query bar
below it. Type a plain-English question into the **ask box** — e.g. *"what is in
my kubernetes notes?"* — and press Enter or click ✨.

**Expect:** the question stays where you typed it. The **query bar below** fills
with a query using real directives, and results follow.

**Read the query.** It should use directives from the help panel (`?`), not
invented ones. A short answer is not a failure: bare words are a valid query —
multiple words mean AND — so *"what do my notes say about incremental reading?"*
becoming `incremental reading` is correct.

## TC-3b Typing a question searches nothing

Type a long question into the **ask box** slowly and watch the results area.

**Expect:** nothing searches, no dropdown appears, and no results change until
you translate. The ask box is inert by design.

*(In the first version the question went into the query bar itself, so every
keystroke ran a literal search and opened the directive autocomplete over the
input. That is why the box was split out.)*

## TC-4 Enter still runs literal text — translation never intercepts

Type a real query by hand — `pod \chapter Kubernetes`, or anything valid for your
collection — and press Enter **without** clicking ✨.

**Expect:** it runs exactly as before. Nothing translates it, nothing rewrites it.

*(This is the guarantee that the feature is additive. If Enter ever routes
through the LLM, the query language has been taken away from you.)*

## TC-5 The generated query is editable

After a translation, edit the query in the bar and press Enter.

**Expect:** your edit runs. The bar is a normal input, not a read-only display.

## TC-6 **The one that matters most — it uses your real chapter names**

Ask about something you know is in a specific chapter, naming it loosely — e.g.
*"what's in my kubernetes notes?"* when your chapter is actually called
`Kubernetes notes`.

**Expect:** the generated query uses **your collection's actual chapter name**,
not an invented one.

*(The prompt is fed your real chapters, contexts and arrow codes. Without them
the model guesses plausible names that match nothing, and you get a query that
looks right and returns nothing — the most likely failure mode this feature has.)*

## TC-7 The hint fetch does not depend on how you got here

This exercises a path that was broken until review caught it.

1. Open the omnibar in **fuzzy** mode and click into the input.
2. Switch to **graph** mode with Ctrl+Shift+K — **without** clicking the input
   again.
3. Type a question and click ✨.

**Expect:** the same quality as TC-6 — real chapter names, not invented ones.

*(Before the fix, the live-vocabulary fetch only fired on input focus in graph
mode. Reaching graph mode by shortcut kept DOM focus, so no second focus event
ever fired and the model was told to "prefer a name from those lists" with no
lists shown.)*

## TC-8 The model picker works and is remembered

Open the picker. Select a specific profile. Translate something.

**Expect:** the list shows "Default model" plus each configured profile by name.
The translation uses the one you picked.

Now **restart the app** and reopen the omnibar in graph mode.

**Expect:** your choice is still selected.

## TC-9 Deleting the chosen profile falls back cleanly

Pick a specific profile, then delete that profile in Settings. Return to the
omnibar.

**Expect:** the picker shows "Default model" and translation still works. It must
**not** keep showing a deleted profile's name, nor fail with a
profile-not-found error.

## TC-10 An unsupported directive is refused, and you can override

Hard to trigger deliberately — the prompt teaches only supported syntax. If you
see a refusal card, check it:

**Expect:** it names what was rejected, shows the offending query, and offers
Retry / Use anyway / Dismiss. **Use anyway** puts the query in the bar regardless.

*(The override exists because the validator can lag a library upgrade. Being
wrong should not be a dead end.)*

## TC-11 A refusal does not follow you around

If you can produce a refusal, then: press Escape to close the omnibar and reopen
it; and separately, toggle to fuzzy mode with Ctrl+K.

**Expect:** the refusal card is gone in both cases. It must not reappear on
reopen, and must never render over the fuzzy panel.

## TC-12 A provider error surfaces without breaking search

Configure a profile with a bad API key or an unreachable endpoint, select it, and
click ✨.

**Expect:** an error appears in the log panel, the ✨ button becomes usable again,
and ordinary graph search still works. The button must not stay stuck in its busy
state.

## TC-13 An empty question does nothing

With the bar empty, click ✨.

**Expect:** nothing happens — no call, no error.

## TC-14 The graph explorer has an ask box too

Open the Graph Explorer. It has an always-visible query bar; the ask box sits
directly above it.

**Expect:** with a profile configured, the ask box is there. Ask *"what is in my
kubernetes notes?"* and press Enter.

**The query fills the bar and does NOT run.** Press Enter in the query bar, or
click ↵, to run it.

*(That is deliberate and differs from the omnibar. The explorer never runs a
query without an explicit submit, and a generated `\notes` scan reads the whole
database — it should not fire on your behalf.)*

With no profile configured, the ask box is absent and the explorer is exactly as
before.

## TC-15 The notebook's graph cell has an ask box too

Add a Graph Query cell to a notebook.

**Expect:** with a profile configured, an ask box sits above the cell's query
input. Ask a question; the query fills the input and **does not run** — press
Enter or **Run**.

Then reopen the notebook.

**Expect:** the translated query persisted, like any query typed into the cell.

Open the same notebook **read-only** (via a gutter dot in the note editor).

**Expect:** no ask box — the query cannot be edited there anyway.

## TC-16 Nothing else changed

Exercise fuzzy search.

**Expect:** unchanged. It gained no ask box.

## What I most want to hear about

- **TC-6 and TC-7** — whether the queries actually use your real names. That is
  the difference between a useful feature and one that confidently returns
  nothing.
- **The quality of the queries themselves**, and which model profile you used.
  A small local model and a frontier one will differ sharply here, and the
  picker exists so you can choose.
- Any query it produces that looks right but returns nothing — that is the
  failure mode validation cannot catch, since it checks vocabulary and not
  whether your collection holds matching data.
