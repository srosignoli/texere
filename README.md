# Texere

A desktop knowledge-management app for N4L plain-text notes: an editor, a
knowledge graph, incremental reading and spaced repetition, built on
[SSTorytime-KV](https://github.com/srosignoli/sstorytime-kv) with
[Bleve](https://blevesearch.com) for full-text search, wrapped in
[Wails](https://wails.io) v2.

Developer setup, build and test instructions live in
[`docs/quickstart.md`](docs/quickstart.md).

---

## Asking in plain language

Graph mode has an **ask box** above the query bar. Type a question there, press
Enter, and an LLM turns it into a query which lands in the query bar — editable,
so you can see exactly what was asked of the graph and adjust it.

The ask box appears only when an LLM profile is configured (Settings → LLM), and
it never searches: nothing typed there runs until it becomes a query.

Two things worth knowing before reading the tables:

- **The query column is exact.** Every query below is verified by
  `internal/corpustest` against real N4L files — each one returns results, not
  merely parses.
- **The question column is illustrative.** What a model actually emits depends on
  the model and on your collection: the prompt is fed your real chapter, context
  and arrow names, so it prefers names you actually have. Treat these as the
  shape to expect, not a guarantee.

A short answer is not a failure. *"what do my notes say about incremental
reading?"* becoming `incremental reading` is a correct translation — bare words
are a valid query.

## One example per query type

| Ask something like | You get a query like | What it does |
|---|---|---|
| what do my notes say about pods? | `pod` | Nodes whose text contains the word |
| pods, but only in my kubernetes chapter | `pod \chapter Kubernetes` | Same, restricted to matching chapters |
| brain waves in the oscillations context | `waves \context oscillations` | Same, restricted to a context tag |
| what chapters do I have? | `\chapter any` | Lists chapters — a table of contents, not a graph |
| which context tags mention oscillations? | `\context oscillations` | Lists matching context tags |
| what relationships are defined? | `\arrow any` | Lists arrow definitions |
| show me everything in my neuroscience notes | `\notes \chapter neuroscience` | The full authored notes for a chapter |
| just the memory parts of my neuroscience notes | `\notes \chapter neuroscience \context memory` | Notes narrowed by context |
| the second page of my cooking notes | `\notes Cooking \page 2` | One page of a `\notes` view |
| how do I get from start to target 1? | `\from start \to target 1` | Paths between two nodes |
| what does door lead to? | `\from door` | Everything downstream of a node |
| what leads to the port node? | `\to port` | Everything upstream of a node |
| tell me the story of Mary | `\story Mary` | An ordered sequence by name |
| what sequences are in my astronomy notes? | `\sequence \chapter astronomy` | Sequences within a chapter |
| what sequences do I have? | `\story any` | Every sequence |
| things near pod, one hop out | `pod \radius 1` | Widens each match's neighbourhood in the explorer |
| fish, but only the first 20 | `fish \limit 20` | Caps how many results come back |

`\notes`, `\story`, `\sequence` and `\from` scan the whole database, so they run
only when you press Enter — they are not fired as you type.

## Asking for a particular kind of text match

Text matching is handled by **Bleve**, the full-text index built alongside the
graph. These operators are what the query bar passes through to it, so phrasing
your question to imply one is how you reach them.

| Ask something like | You get a query like | Bleve behaviour |
|---|---|---|
| notes about running | `run` | Whole words, stemmed — matches *running*, *runs*; case- and accent-insensitive |
| notes mentioning recipe, fish and soup | `recipe fish soup` | Multiple words are ANDed — all must appear |
| brain but not waves | `brain&!wave` | `&` is AND, `!` is NOT, `\|` is OR (`!` binds tightest, then `&`, then `\|`) |
| the exact word pod, not pods | `!pod!` | Exact word, bypassing the stemmer — `\|pod\|` works too |
| anything starting with neuro | `neuro:*` | Begins-with on the stem — matches *neuroscience*, *neurology* |
| the phrase "thank you" | `"thank you"` | Adjacent and in order, not merely both present |
| fángzi, but I can't type the accent | `(fangzi)` | Parentheses fold diacritics; CJK text such as `房子` matches directly |

The node-path shorthand `<a|b>` is equivalent to `\from a \to b`, and
`<a|context|b>` adds a context. Angle brackets are reserved for that form —
anything else in `<…>` is refused with an explanation.

## Where this is defined

The query language has one source of truth,
[`frontend/src/lib/query-grammar.js`](frontend/src/lib/query-grammar.js). The
autocomplete dropdown, the in-app help panel (`?` beside the query bar) and the
LLM's prompt are all generated from it, so the three cannot drift apart. The
tables above are written by hand and are the exception — if they disagree with
the help panel, the help panel is right.
