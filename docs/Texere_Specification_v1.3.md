# N4L Workbench — System Specification

**v1.3 — April 2026 | Confidential**

A Comprehensive Personal Knowledge Management and Incremental Learning System Built on N4L and SSTorytime.

---

## Table of contents

1. [Executive summary](#1-executive-summary)
2. [Interface architecture](#2-interface-architecture)
3. [Operational modes](#3-operational-modes)
4. [N4L syntax: native features and extensions](#4-n4l-syntax-native-features-and-extensions)
5. [Storage architecture](#5-storage-architecture)
6. [Scheduling architecture](#6-scheduling-architecture)
7. [Automatic priority management](#7-automatic-priority-management)
8. [Advanced statistics](#8-advanced-statistics)
9. [Knowledge Architect (tutor tab)](#9-knowledge-architect-tutor-tab)
10. [Source provenance and incremental reading](#10-source-provenance-and-incremental-reading)
11. [Graph explorer (explore mode)](#11-graph-explorer-explore-mode)
12. [Neural review mode](#12-neural-review-mode)
13. [Technology considerations](#13-technology-considerations)
14. [Version control and sync](#14-version-control-and-sync)
15. [Complete N4L example](#15-complete-n4l-example)
- [Appendix A: Complete syntax reference](#appendix-a-complete-syntax-reference)
- [Appendix B: Glossary](#appendix-b-glossary)

---

## 1. Executive summary

The N4L Workbench is a personal knowledge management and incremental learning system that extends Mark Burgess's SSTorytime/N4L ecosystem. It combines a Squeak-inspired structural browser with a source-first text editor, three specialized scheduling engines, an AI-powered tutoring companion, an interactive graph explorer, a neural review mode for creative knowledge exploration, a rich read view for comfortable note reading, and integrated version control for tracking knowledge evolution.

The system is fully self-contained: a single Go binary embedding SSTorytime-KV (BadgerDB) for the knowledge graph, SQLite for learning state, and go-git for version control, with zero external dependencies. No database servers, no Docker, no credentials, no git CLI required. Download, point at your notes folder, run.

### 1.1 Design principles

- **Source-first:** the .n4l plain text file is the single source of truth for knowledge content
- **Self-contained:** one Go binary, zero external dependencies, full offline operation
- **Separation of concerns:** knowledge content (N4L files), knowledge graph (BadgerDB), learning state (SQLite)
- **Pleasure-driven learning:** variety, agency, and visible progress make studying enjoyable
- **Transparency:** every automatic decision is visible and overridable
- **Portability:** the entire collection is a folder you can zip, version-control, and move between machines
- **Version-controlled:** integrated git tracks knowledge evolution with automatic meaningful commits on each build, optional remote sync to GitHub/GitLab
- **Full N4L fidelity:** context operators (+::, -::), sequence mode, labels, references, SSTconfig arrow definitions, annotation symbols, ALL CAPS reminders

---

## 2. Interface architecture

The workbench uses a hybrid layout combining Squeak-style structural navigation with source-first editing. Three horizontal bands support four operational modes plus a Knowledge Architect tutor and a read view.

### 2.1 Layout bands

#### 2.1.1 Top band: structural navigation + orbit minimap

- **Column 1 — Chapter/File:** Lists all chapters from open .n4l files. Selecting a chapter filters Column 2.
- **Column 2 — Context tags (faceted filter):** Shows all unique context tags from the selected chapter as toggleable chips. The parser computes the effective context set at each line by replaying `::`, `+::`, `-::` operators. The `_sequence_` tag receives distinct visual treatment (chain icon).
- **Column 3 — Nodes:** Root nodes matching the active context filter. In `_sequence_` contexts, nodes display in sequence order with connector lines. Otherwise file order with optional alphabetical sort.
- **Column 4 — Orbit minimap:** Radius-1 graph neighbors with typed edges, powered by SSTorytime-KV (BadgerDB). For `_sequence_` nodes, neighbors display as a horizontal chain with non-sequence links branching vertically. Shows the last-built graph state.

#### 2.1.2 Middle band: context-sensitive button bar

Core actions: +node, +link, +context, save (Ctrl+S), build graph (Ctrl+G), search, undo, study, Go Neural. Additionally:

- **Reminder counter badge:** Shows open ALL CAPS reminders count (amber). Clicking opens the reminder panel.
- **Arrow palette (+link dropdown):** Grouped by SST meta-type, recently used arrows at top. Powered by SSTconfig arrow registry.
- **Build graph indicator:** Shows "N files modified since last build" next to the Build button. Clears on build. Build also triggers a git auto-commit.
- **Sync button:** Pushes committed changes to the configured remote repository (GitHub/GitLab). Shows a spinner during push. Only visible when a remote is configured.
- **History button:** Opens the version history panel showing the commit log with auto-generated build summaries. Click a commit to see the diff.

#### 2.1.3 Bottom band: editor area with three tabs

- **Source tab (default):** Full N4L editor with CodeMirror 6, syntax highlighting for all native and extended syntax, arrow autocomplete, and validation.
- **Read tab:** Rich read-only rendered view of the current file. Media (images, audio, PDF thumbnails) displayed inline. Nodes rendered as structured cards with arrows resolved into labeled properties. Reminders highlighted as amber alerts. Sequence nodes connected visually.
- **Tutor tab:** Knowledge Architect AI-powered Socratic tutoring interface with LLM selector.

### 2.2 Bidirectional sync model

- **Columns → Editor:** Clicking a node scrolls the editor to that line. Orbit and read view update.
- **Editor → Columns:** Cursor movement updates Column 2 tag highlights, Column 3 selection, orbit, and read view scroll position.

### 2.3 Reminder panel

- **Display:** Lists all ALL CAPS lines across open files with file, line number, and age.
- **Actions:** Click to jump to line. "Resolve" to lowercase the text. "Discuss" to open in Knowledge Architect.
- **Scheduling integration:** Open reminders boost topic curation priority. Stats dashboard tracks knowledge debt.

### 2.4 Read view

The read view is a third tab alongside Source and Tutor, providing a rich rendered presentation of the current file:

- **Nodes as cards:** Each node displays as a styled card with the node text as heading. Ditto chains are resolved — arrows display as labeled properties (e.g., "means: fast, intuitive thinking") rather than raw ditto syntax.
- **Images:** Rendered inline at configurable size. Click to open larger preview. File paths resolved relative to the .n4l file.
- **Audio:** Compact inline player (play button + filename). Click to play.
- **PDF references:** `(from)` arrows to local PDFs render a page thumbnail. Click to open at the referenced page.
- **Reminders:** ALL CAPS lines render as amber alert bars.
- **Sequences:** Lines in `_sequence_` contexts display in order with visual connectors.
- **Context transitions:** `::`, `+::`, `-::` render as section dividers with context tags as pills.
- **FSRS badges:** Each node with a study marker shows a subtle dot indicating FSRS state (green=mature, amber=due, red=leech).
- **Interactivity:** Clicking a node jumps the source tab to that line. Clicking an arrow target opens explore mode centered on that node. The read view is strictly read-only — all editing happens in the source tab.

---

## 3. Operational modes

Four modes, each providing a different lens on the same data. All modes read from the same SSTorytime-KV (BadgerDB) and learning.db (SQLite) stores.

### 3.1 Edit mode (default)

Primary mode for writing and curating. Editor maximized in the bottom band. Orbit minimap provides radius-1 context from BadgerDB. Arrow autocomplete, validation, and the arrow palette are active. Media preview panels appear on `(audio)`/`(image)` lines. The read view tab offers a comfortable reading alternative.

### 3.2 Explore mode

The graph explorer replaces the editor area with a full interactive Cytoscape.js graph canvas and a query bar for cross-collection search. See Section 11.

### 3.3 Study mode

Interleaved learning session drawing from the unified priority queue. Three element types (topics, extracts, items) in a configurable mix. Neural review accessible as a sub-mode via "Go Neural." See Section 6 and Section 12.

### 3.4 Stats mode

Read-only dashboard with topic, extract, item, and neural review statistics. See Section 8.

---

## 4. N4L syntax: native features and extensions

### 4.1 Native N4L syntax

#### 4.1.1 Chapters

```n4l
- my notes                      # chapter title
```

#### 4.1.2 Context operators (state machine)

| Operator | Syntax | Effect |
|---|---|---|
| Set (replace) | `:: tag1, tag2` | Replaces the active context set entirely |
| Extend (add) | `+:: tag3, tag4 ::` | Adds tags to the current active set |
| Prune (remove) | `-:: tag3 :` | Removes tags from the current active set |

The active context set at any line is computed by replaying all operators from the chapter start. Every node inherits the effective context set at its position.

#### 4.1.3 Sequence mode

The special context tag `_sequence_` creates "leads to" (LT) links between consecutive lines. Combinable with other tags: `:: _sequence_, french, story`.

#### 4.1.4 Nodes, arrows, and ditto chains

```n4l
 petit-déjeuner (means) breakfast
     "  (e.g.) Je voudrais le petit-déjeuner
```

Arrow types must be registered in SSTconfig. See Section 4.2.

#### 4.1.5 Labels and references

```n4l
@mylabel foot (note) important concept
$mylabel.1 (eh) 脚    $PREV.3 (e.g.) example
```

#### 4.1.6 Comments, reminders, and annotation symbols

```n4l
# this is a comment
HOW TO SPELL VONBRAUNS NAME???    # ALL CAPS = reminder
The >referendum was controversial  # >> = annotation symbol
```

Annotation symbols (`%`, `=`, `**`, `>>`) are defined in `SSTconfig/annotations`. They mark substrings as specific arrow types and receive distinct syntax highlighting.

### 4.2 SSTconfig arrow definitions

The SSTconfig directory contains files organized by four SST meta-types plus annotations. The workbench reads these at startup and builds an arrow registry. Arrow definition format:

```
+ has example       (e.g.)    - is an example of     (ex)
+ note/remark       (note)    - is a note about      (isnotefor)
+ leads to          (fwd)     - comes from           (bwd)
  looks like        (ll)         # no pair, one-way
```

Each arrow has: a `+` (forward) and optionally `-` (reverse) direction, a long name, a short code in parentheses, and context groupings within the file (`:: motion, action ::` etc.).

#### 4.2.1 Arrow-aware editor features

- **Inline autocomplete:** Triggers on typing `(`. Shows short code, long name pair, SST meta-type, and ↔/→ icon. Context-aware and ditto-aware.
- **Arrow palette:** +link button bar dropdown, grouped by meta-type, recently used at top.
- **Arrow reference panel:** Collapsible sidebar with all arrows, usage stats, and + new arrow dialog.
- **Validation:** Unregistered arrows get amber underline with quick-fix to register.

#### 4.2.2 Arrow pairs and study card generation

- Arrows with a `+`/`-` pair automatically generate bidirectional cards. No `# @card reverse` needed.
- Arrows without a pair generate one-way cards only.
- `# @card reverse` forces bidirectional on a one-way arrow. `# @card no-reverse` suppresses reverse on a paired arrow.

#### 4.2.3 Study metadata extension

```
+ has example  (e.g.)  # studyable: true  cloze-eligible: true
+ note/remark  (note)  # studyable: false  hint-on-reveal: true
```

Optional metadata stored as comments in SSTconfig. Editable via the arrow reference panel.

### 4.3 Study markers (extension)

#### 4.3.1 Individual markers

| Marker | Scope | Effect |
|---|---|---|
| `# @card` | Next line | Card (arrow pair determines direction) |
| `# @card reverse` | Next line | Force bidirectional on one-way arrow |
| `# @card no-reverse` | Next line | Suppress reverse on paired arrow |
| `# @cloze` | Next line | Cloze deletion from `{braces}` |

#### 4.3.2 Block markers

| Marker | Effect |
|---|---|
| `# @study` | All nodes become cards (pairs determine direction) |
| `# @study reverse` | Also reverse one-way arrows within block |
| `# @study sequence` | Also generate sequence recall cards |
| `# @end` | Closes the study block |

#### 4.3.3 Rules

- Ditto lines inherit parent's study marker. `{braces}` in blocks trigger cloze. Individual markers override block defaults.
- `(note)` and arrows with `studyable: false` become hints, never tested.
- Blocks close at `# @end`, new `# @study`, `::` reset, or EOF. `+::` and `-::` do NOT close blocks.

#### 4.3.4 Sequence cards

In `_sequence_` contexts: forward ("What comes after X?"), gap ("What is missing?"), and order ("Place in order") variants.

### 4.4 Media, priority, and provenance

```n4l
    "  (audio) "media/fr/petit-dejeuner.mp3"
    "  (image) "media/fr/petit-dejeuner.jpg"
# @priority 15                    # 0-100, 0=highest
# @source "Thinking, Fast and Slow" by Daniel Kahneman
# @ref "sources/kahneman-tfs.pdf"
# @progress ch.3 p.47
System 1 operates automatically (from) "ch.1 p.20"
```

### 4.5 Parser model

The parser maintains a running `effective_context` set per chapter (`::` replaces, `+::` unions, `-::` subtracts). Each node stores a snapshot. Syntax highlighting: `::` amber, `+::` green, `-::` coral, `_sequence_` blue. Annotation symbols receive distinct highlighting with hover tooltips.

---

## 5. Storage architecture

Three storage layers, all embedded, zero external dependencies.

### 5.1 .n4l source files

Source of truth for knowledge content. Human-readable, version-controllable. The only file the learning engine writes to is optional: lowercasing resolved reminders and updating `# @progress` markers.

### 5.2 SSTorytime-KV (BadgerDB)

A port of SSTorytime that replaces PostgreSQL with BadgerDB — an embedded, serverless key-value store. Same API as the original SSTorytime (searchN4L, pathsolve, orbit queries), but everything runs in-process. No PostgreSQL installation, no Docker, no credentials.

- **Updated via Build graph (Ctrl+G):** Runs the N4L compiler on all modified files and writes to BadgerDB. Takes ~100–500ms for typical collections.
- **Powers:** orbit minimap, graph explorer, search, pathsolve, neural review activation, graph analysis.
- **Not updated on file save:** The graph reflects the last-built state. Half-written nodes don't pollute search results. Building is an intentional act: "this knowledge is ready."

### 5.3 learning.db (SQLite)

Embedded relational database (`modernc.org/sqlite`, pure Go) for learning state. Optimized for the filtering, sorting, and aggregation that FSRS scheduling requires.

#### 5.3.1 Schema

| Table | Purpose | Key columns |
|---|---|---|
| items | FSRS state per element | node_id, file, chapter, effective_context (JSON), type, difficulty, stability, retrievability, next_review, priority, is_sequence_member |
| review_logs | Complete review history | item_id, timestamp, grade, elapsed_ms, old_stability, new_stability, card_type (basic/reverse/cloze/sequence) |
| priority_history | Auto-priority audit trail | item_id, timestamp, old_priority, new_priority, reason |
| sources | Registered source materials | source_id, title, author, ref_path, ref_url, progress, chapter |
| tutor_sessions | Knowledge Architect logs | session_id, chapter, context_tags, llm_model, messages_json, created |
| daily_stats | Aggregated daily metrics | date, reviews_count, topics_reviewed, cards_added, avg_grade, time_spent_s |
| reminders | ALL CAPS reminder tracking | reminder_id, file, line, text, status (open/resolved), created, resolved_at |
| neural_sessions | Neural review logs | session_id, seed_node, strategy, nodes_visited, retrievals_rescued, created |

#### 5.3.2 Sync boundary

- **Save file (Ctrl+S) → SQLite:** Parser scans for study markers, compares with database, creates/deactivates FSRS entries. Updates reminder tracking. Fast (~10–50ms).
- **SQLite → N4L files:** Never (except optional `# @progress` and reminder resolution).

### 5.4 SSTconfig directory

Read at startup. Contains arrow definition files (leads_to, contains, properties, similarity, annotations). The workbench can write new definitions via the + new arrow dialog.

### 5.5 Why two embedded databases

BadgerDB and SQLite serve genuinely different data shapes. BadgerDB excels at graph traversal, key-value lookups, and prefix iteration — ideal for the knowledge graph. SQLite excels at filtered queries, sorted results, and aggregation — essential for the study queue builder (`WHERE next_review <= now() ORDER BY priority`), the FSRS optimizer (20K+ review rows as matrix input), and the stats dashboard (`GROUP BY` aggregations). The Go backend reads from both and composes results for the frontend.

### 5.6 Folder structure

```
my-notes/
  french_phrases.n4l
  chinese_learning.n4l
  SSTconfig/
    leads_to
    contains
    properties
    similarity
    annotations
  media/
    fr/petit-dejeuner.mp3
  sources/
    kahneman-tfs.pdf
  badger/                  # SSTorytime-KV data (auto-created)
  learning.db              # FSRS + learning state (auto-created)
  .git/                    # version history (auto-initialized)
  .gitignore               # excludes badger/ and learning.db
```

### 5.7 Version control layer

A git repository (managed via go-git) tracks the evolution of .n4l files, SSTconfig/, and media/. The `badger/` directory and `learning.db` are .gitignored because they are binary, machine-specific, and rebuildable. Version control is tightly integrated with the Build graph action — see Section 14 for full details.

---

## 6. Scheduling architecture

Three distinct scheduling engines unified by a single priority queue. FSRS only applies to items.

### 6.1 Item scheduling (FSRS v6)

21 model weights, desired retention default 0.9, four grades (Again/Hard/Good/Easy). Four card types tracked independently: basic, reverse (driven by arrow pair definitions), cloze, and sequence. FSRS optimizer runs automatically after ~500 reviews.

### 6.2 Topic scheduling (curation-driven)

Signals: child health (avg retrievability of child cards), staleness (days since last edit), unprocessed ratio, growth signal, open reminders count, manual `# @priority`.

```
next_review = last_review + (base_interval × A_factor)
```

A_factor adjusted by signals. Rating: "needs work" vs. "good shape." When a topic surfaces, the editor opens at the relevant section.

### 6.3 Extract scheduling (distillation-driven)

```
next_review = now + (base_interval × 1.5 ^ deferral_count)  — max 90 days
```

Actions: promote to @card, mark @cloze, refine, defer, dismiss, split.

### 6.4 Unified priority queue

All three types share a 0–100% queue. Default mix: 20% topics, 15% extracts, 65% items. Adapts to collection state. Auto-postpone defers low-priority elements when the queue exceeds the time budget. Configurable randomization prevents tunnel vision.

---

## 7. Automatic priority management

### 7.1 Item priority rules

- **Leech detection:** 5+ lapses → priority boost + reformulation flag
- **Stability demotion:** S > 180d → gentle lowering
- **Prerequisite boosting:** Weak dependency (including sequence "leads to" links) boosts the prerequisite
- **Context coherence:** Siblings sharing effective context tags get temporary boost
- **Decay toward baseline:** Auto-adjustments drift back toward manual `# @priority`

### 7.2 Topic priority rules

- **Crisis boost:** Many lapsed child cards
- **Staleness creep:** Not edited in 30+ days
- **Coverage gap:** < 30% nodes carded
- **Growth surge:** New nodes added recently
- **Open reminders:** ALL CAPS reminders boost curation urgency

### 7.3 Extract priority rules

- **Hub node:** High graph centrality
- **Prerequisite demand:** Dependent card is a leech
- **Sequence position:** Earlier `_sequence_` nodes before later ones
- **Deferral decay:** Each "not yet" lowers priority exponentially

### 7.4 Transparency

Every adjustment logged in `priority_history` with timestamp, values, and human-readable reason. Visible in stats dashboard and during study sessions.

---

## 8. Advanced statistics

### 8.1 Topic statistics

Active topics, average staleness, coverage, topics in crisis, open reminders (knowledge debt), curation timeline visualization.

### 8.2 Extract statistics

Pending count, promotion rate, dismiss rate, avg deferrals, distillation funnel visualization.

### 8.3 Item statistics

Active cards, avg retrievability, leech count, mature count, forgetting curves by chapter, review heatmap (12 weeks), stability histogram, FSRS accuracy, topic mastery bars, card type breakdown.

### 8.4 Neural review statistics

Sessions count, average depth (hops from seed), strategy usage distribution, discovery rate (nodes not seen in 30+ days), retrieval rescue count.

---

## 9. Knowledge Architect (tutor tab)

### 9.1 Core principle

The tutor never writes flashcards. It challenges understanding and critiques the user's drafted formulations using the Feynman Technique and Wozniak's 20 Rules.

### 9.2 Context injection

The LLM receives: current chapter and effective context set, selected node and orbit neighbors (from BadgerDB), media references, existing study items, FSRS weakness data, source material via `(from)` arrows, the full sequence chain if in `_sequence_` context, and the activation path if entering from neural review.

### 9.3 Session management

- Sessions persist per topic in `tutor_sessions` table
- LLM model selectable via dropdown (API models + custom local endpoints)
- Reminder conversion: ALL CAPS reminders can be sent directly as conversation starters
- Arrow validation: tutor warns about unregistered arrows when user drafts N4L in chat

### 9.4 Integration

- **Source → Tutor:** "Send selection" and "Send full context"
- **Tutor → Source:** Purple margin dots on discussed lines. "Push to source" inserts approved N4L.

---

## 10. Source provenance and incremental reading

### 10.1 Provenance chain

- **Source → Extract:** `(from)` arrows with free-form location strings
- **Extract → Card:** Natural via ditto chains and `@card`/`@cloze` markers

### 10.2 UI integration

- **Edit mode:** Source preview panel on `(from)` lines. "Extract from source" button.
- **Read view:** `(from)` references to local PDFs render as inline page thumbnails.
- **Explore mode:** Source references as distinct styled nodes in the graph.
- **Study mode:** "View source" link on cards with provenance.
- **Knowledge Architect:** Source passages auto-included in LLM context.

### 10.3 Sequence mode and incremental reading

`_sequence_` preserves reading order while enabling graph annotations. Combined with `(from)` arrows and `# @progress`, it creates the complete incremental reading pipeline. The read view renders sequences with visual connectors and inline source thumbnails.

---

## 11. Graph explorer (explore mode)

Explore mode replaces the editor area with a full interactive graph explorer powered by Cytoscape.js, querying SSTorytime-KV (BadgerDB). This is a dedicated mode for cross-collection discovery and search, distinct from the editor's local orbit minimap.

### 11.1 Library: Cytoscape.js

Selected for: built-in graph algorithms (BFS, PageRank, betweenness centrality), multiple layout algorithms, typed-edge customization, interactive pan/zoom/click, MIT license, and strong performance at 500–2000 nodes.

### 11.2 Interface

- **Query bar:** Accepts SSTorytime search syntax (searchN4L). Results render as a connected subgraph. "Paths" button switches to pathsolve mode for finding paths between two selected nodes. Queries search across the entire collection, not just the current file.
- **Arrow-type filter chips:** Toggle edge visibility by SST meta-type. Filter to "leads to" only → see causal skeleton. Filter to "similar" only → see analogy web.
- **Graph canvas:** Interactive Cytoscape.js with pan, zoom, box-select, drag.
- **Node detail panel:** Right sidebar: N4L content, all arrows, FSRS state, source provenance, three actions — edit (jump to source), study (test card now), neural (Go Neural from this node).

### 11.3 Visual encoding

- **Edge color by meta-type:** Purple = leads to, coral = property, teal = contains, amber = similar, blue = sequence LT.
- **Node opacity by retrievability:** Bright = well-known, faded = approaching forgetting.
- **Node ring by status:** Green = mature, amber = due, red = leech, gray = unmarked.
- **Node size by centrality:** Optional PageRank/betweenness sizing.

### 11.4 Layout algorithms

- **Force-directed (CoSE):** Default. Reveals clusters.
- **Hierarchical (Dagre):** For directional relationships.
- **Circular:** Neighborhood with seed at center.
- **Grid:** Side-by-side comparison.

### 11.5 Unique capabilities

- **Path visualization:** pathsolve results highlighted as colored chains with arrow types visible.
- **Centrality highlighting:** Hub concepts identified. High-centrality + low-retrievability = priority crisis.
- **Cluster detection:** Force-directed layout reveals unexpected groupings across chapters.
- **Temporal layering:** Nodes colored by creation date or last review.

### 11.6 Workflow integration

- Click node → see FSRS status + edit/study/neural actions
- Visual scanning for weak areas (faded nodes = forgotten knowledge)
- Right-click node in orbit (edit mode) → "Show in graph" → switch to explore mode
- Click "edit" on any node in explore → switch to edit mode at that source line
- All queries powered by the same BadgerDB as the orbit — no sync differences

---

## 12. Neural review mode

Neural review uses typed spreading activation over the SSTorytime-KV graph for directed knowledge exploration. Inspired by SuperMemo's neural creativity, but more powerful because N4L's typed arrows enable semantically directed strategies.

### 12.1 Spreading activation algorithm

Starting from a seed node, activation spreads along graph edges with strength decaying per hop (default 0.7). Focusing a new node triggers further spreading, growing the queue organically.

- **Arrow type weight:** Strategy determines which arrow types conduct activation.
- **Arrow pair directionality:** Forward (+) weighted higher by default. Toggle for reverse exploration.
- **FSRS retrievability:** Low-retrievability nodes get boosted — the walk gravitates toward knowledge you're about to forget.
- **Context relevance:** Nodes sharing context tags with seed get boosted.
- **Priority and randomization:** `# @priority` modulates. Configurable randomization prevents determinism.

### 12.2 Five exploration strategies

| Strategy | Primary arrows | Question | Best for |
|---|---|---|---|
| Causal walk | leads to (fwd, next, causes) | What follows? | Processes, stories |
| Decomposition drill | contains (consist, has-pt) | What's inside? | Systems, structures |
| Property map | properties (means, e.g.) | What do I know? | Vocabulary, facts |
| Analogy web | similarity (ll, !confuse) | What's like this? | Mnemonics, false friends |
| Free exploration | All types, balanced | What's connected? | Creativity, serendipity |

### 12.3 Interface

- **Strategy selector:** Chip bar at top.
- **Live activation graph:** Uses the Cytoscape.js canvas. Node opacity = activation strength.
- **Node detail + actions:** Review (FSRS grade), edit (jump to source), discuss (Knowledge Architect with activation path as context).
- **Neural queue:** Bottom bar with upcoming nodes ordered by activation.

### 12.4 Integration

- Accessible from study mode ("Go Neural") or explore mode (right-click → "Explore from here")
- Reviews during neural exploration count as normal FSRS grades
- Knowledge Architect receives the activation path as context
- Exiting returns to previous mode with queue intact

---

## 13. Technology considerations

### 13.1 Performance targets

| Operation | Target | Notes |
|---|---|---|
| N4L parse + context replay | < 50ms / 10K lines | Single-pass |
| SSTconfig registry build | < 10ms | Read once at startup |
| Build graph (Ctrl+G) | 100–500ms | BadgerDB batch write |
| Orbit query (radius 1) | < 1ms | BadgerDB key lookup |
| Graph explorer render | < 500ms | Cytoscape.js initial layout |
| Study queue build | < 100ms | SQLite indexed query |
| Neural activation per hop | < 50ms | BFS with weighted edges |
| FSRS optimizer | < 30s / 20K reviews | Background thread |
| Read view render | < 200ms | HTML from parsed AST |

### 13.2 Technology stack

| Component | Technology | Role |
|---|---|---|
| Application framework | Wails v2 (Go + OS webview) | Go backend + web frontend, ~10-15MB binary |
| Source editor | CodeMirror 6 | N4L syntax highlighting, autocomplete, line tracking |
| Graph visualization | Cytoscape.js | Explorer canvas, neural review graph, orbit minimap |
| Knowledge graph | SSTorytime-KV (BadgerDB) | Embedded graph store, search, pathsolve (pure Go) |
| Learning state | SQLite (modernc.org/sqlite) | FSRS scheduling, review history, stats (pure Go) |
| Version control | go-git (pure Go) | Auto-commit on build, remote sync, history (no git CLI needed) |
| FSRS algorithm | go-fsrs or ts-fsrs | Scheduler + optimizer |
| Stats charts | Chart.js | Dashboard visualizations |
| Frontend framework | TBD (Svelte recommended) | Component structure for webview UI |

The entire application compiles to a single Go binary with zero external dependencies. Pure Go SQLite (`modernc.org/sqlite`), pure Go BadgerDB, and pure Go go-git eliminate CGo entirely, enabling trivial cross-compilation to Windows and Linux. The user does not need git installed on their system.

---

## 14. Version control and sync

The workbench integrates lightweight git version control via go-git (pure Go). This provides automatic versioning of knowledge evolution, optional remote sync to GitHub/GitLab, and a history viewer — without requiring the user to install git or learn command-line tools.

### 14.1 Design philosophy

The integration is Level 1: auto-commit on build with optional sync. It is not a full git client. Users who need branch management, merge conflict resolution, or interactive rebase should use their preferred git tool on the same repository — the auto-commits from the workbench are standard git commits.

### 14.2 Initialization

On first run, if no `.git` directory exists in the notes folder, the workbench offers to initialize version control. This creates a `.git` repository and a `.gitignore` file:

```gitignore
# Auto-generated by N4L Workbench
badger/
learning.db
learning-export.json
*.db-journal
*.db-wal
```

The `.gitignore` excludes BadgerDB data and SQLite databases because they are binary, machine-specific, and can be rebuilt from the .n4l source files. Only knowledge content (.n4l files), configuration (SSTconfig/), and media assets (media/) are versioned.

### 14.3 Auto-commit on build

When the user presses Build graph (Ctrl+G), after the BadgerDB write completes, the workbench automatically stages and commits:

- **Staged files:** All .n4l files, SSTconfig/ directory, media/ directory.
- **Commit message:** Auto-generated from the build context. Example: "Build: 3 files changed, +12 nodes, +4 cards, 2 reminders resolved." The N4L compiler already computes these statistics during the build.
- **No commit if nothing changed:** If git detects no modifications since the last commit, the commit step is silently skipped.

This ties version snapshots to a meaningful action — each commit represents a coherent state of knowledge that the user has declared ready, not a half-written edit. The auto-generated messages create a rich history without requiring manual discipline.

### 14.4 Remote sync

- **Sync button:** Visible in the button bar when a remote is configured. Clicking pushes all committed changes to the remote repository in a background goroutine. A spinner indicates progress. Push failures show an amber notification without blocking the UI.
- **Auto-sync option:** In settings, the user can enable "auto-sync on build" which pushes immediately after each auto-commit.
- **Configuration:** Settings panel fields: remote URL (HTTPS), authentication token (personal access token for GitHub/GitLab). SSH keys are not supported to keep the implementation simple and avoid system-level key management.
- **Conflict handling:** If the remote has diverged (e.g., edits on another machine that weren't pulled), the sync fails with a clear message: "Your notes have diverged from the remote. Please resolve this using your preferred git tool and restart." N4L files are line-based plain text, so git's standard merge usually handles concurrent edits well.

### 14.5 Version history panel

Accessible from the History button in the button bar, or by right-clicking a file in Column 1:

- **Commit log:** Scrollable list showing each build commit with auto-generated message, timestamp, and list of changed files.
- **Diff view:** Clicking a commit shows a standard diff of the changes (added lines in green, removed in red). Useful for tracking how a specific node or chapter evolved over time.
- **File-level history:** Right-clicking a file in Column 1 and selecting "History" shows only the commits affecting that file.
- **Restore:** Clicking "Restore this version" on a historical commit checks out that version of the file to a temporary location for review. The user can then manually copy content back into the current file. No automatic revert to avoid accidental data loss.

### 14.6 Learning state sync across devices

The `learning.db` (SQLite) is .gitignored because it is binary and machine-specific. For multi-device FSRS sync, a complementary export/import mechanism is provided:

- **Export:** Settings → "Export learning state" writes a `learning-export.json` file containing all `review_logs` entries since the last export. This file is text-based and small (a few KB per session).
- **Import:** Settings → "Import learning state" reads the export file and merges review entries by timestamp. Since review logs are append-only, the merge is conflict-free: take the union of both machines' review histories for each item, then recompute FSRS state from the merged history.
- **Automation (optional):** Auto-export on sync, auto-import on pull. The export file can be committed alongside .n4l files for transport, then deleted after import.

### 14.7 Settings

| Setting | Default | Description |
|---|---|---|
| Enable version control | true | Auto-commit on build |
| Remote URL | (empty) | HTTPS URL for GitHub/GitLab remote |
| Auth token | (empty) | Personal access token for remote push |
| Auto-sync on build | false | Push to remote immediately after each build |
| Auto-export learning state | false | Export FSRS data on each sync for multi-device use |

---

## 15. Complete N4L example

```n4l
# @source "Thinking, Fast and Slow" by Daniel Kahneman
# @ref "sources/kahneman-tfs.pdf"
# @progress ch.3 p.47
# @priority 20

- Thinking Fast and Slow — reading notes

:: _sequence_, cognitive science

# @study
System 1 operates automatically (from) "ch.1 p.20"
    "  (means) fast, intuitive, effortless thinking
    "  (audio) "media/kahneman/system1.mp3"
# @cloze
    "  (e.g.) Detecting that one object is {farther} than another
    "  (note) contrast with System 2

System 2 allocates attention to effort (from) "ch.1 p.21"
    "  (means) slow, deliberate, logical thinking
# @card
    "  (e.g.) Filling out a tax form

CHECK IF SYSTEM 3 EXISTS IN LATER CHAPTERS???
# @end

-:: _sequence_ :
+:: biases ::

@anchor1 anchoring effect (from) "ch.11 p.119-122"
    "  (means) relying too heavily on first information
# @card no-reverse @priority 5
    "  (e.g.) salary negotiation: first number sets the range
    "  (note) Kahneman: adjustment is always insufficient
    "  (image) "media/kahneman/anchoring-diagram.png"
    "  (!confuse) availability heuristic

$anchor1.1 (fwd) availability heuristic
```

This demonstrates: source registration with progress, chapter priority, `_sequence_` + topic tags, study block, cloze/card overrides, media (audio/image), provenance arrows, ALL CAPS reminder, context transitions (`-::`/`+::`), labels/references, no-reverse override, similarity link (`!confuse`), leads to link (`fwd`), and SSTconfig arrow codes. The read view would render this with inline audio player, image thumbnail, amber reminder alert, and resolved ditto chains.

---

## Appendix A: Complete syntax reference

### Native N4L syntax

| Syntax | Type | Description |
|---|---|---|
| `- title` | Chapter | Declares a chapter |
| `:: tag1, tag2` | Context | Sets (replaces) active context |
| `+:: tag3 ::` | Context | Adds tags to active context |
| `-:: tag2 :` | Context | Removes tags from active context |
| `_sequence_` | Context tag | Creates "leads to" chains |
| `node (arrow) target` | Node + link | Node with typed arrow |
| `"  (arrow) target` | Ditto | Refers to most recent non-ditto node |
| `@label text` | Label | Named anchor |
| `$label.N` / `$PREV.N` | Reference | Cross-reference |
| `# comment` | Comment | Not uploaded to graph |
| `ALL CAPS TEXT` | Reminder | Open loop / knowledge debt |
| `(arrow) "url/path"` | URL target | External resource reference |
| `%`  `=`  `>>`  `**` | Annotation | Inline substring markers |

### Workbench extensions

| Syntax | Type | Description |
|---|---|---|
| `# @card` | Study | Card (pair determines direction) |
| `# @card reverse` | Study | Force bidirectional |
| `# @card no-reverse` | Study | Suppress reverse |
| `# @cloze` | Study | Cloze deletion |
| `# @study` / `reverse` / `sequence` | Study | Study block |
| `# @end` | Study | Closes block |
| `{text}` | Study | Cloze marker (inline) |
| `# @priority N` | Priority | 0–100 (0=highest) |
| `# @source "title"` | Provenance | Source material |
| `# @ref "path/url"` | Provenance | Source location |
| `# @progress loc` | Provenance | Reading position |
| `(audio)` / `(image) "path"` | Media | Media reference |
| `(from) "location"` | Provenance | Source location reference |

---

## Appendix B: Glossary

| Term | Definition |
|---|---|
| A-factor | Topic review interval multiplier |
| Activation | Spreading signal strength in neural review |
| Annotation symbol | Inline character marker (`%`, `=`, `>>`) from SSTconfig |
| Arrow pair | Forward (+) and reverse (-) definitions in SSTconfig |
| Arrow registry | In-memory index of SSTconfig arrows, built at startup |
| Auto-commit | Automatic git commit triggered by Build graph (Ctrl+G) |
| Auto-postpone | Silently defers low-priority elements when queue exceeds budget |
| BadgerDB | Embedded key-value store powering SSTorytime-KV |
| Build graph | Ctrl+G: compile N4L files into SSTorytime-KV + auto-commit |
| Desired retention | FSRS target recall probability (default 0.9) |
| Difficulty (D) | FSRS variable (1–10) per card |
| Distillation | Converting extracts into study items |
| Effective context | Computed tag set by replaying `::`, `+::`, `-::` from chapter start |
| Extract | Unmarked node in the distillation pipeline |
| FSRS | Free Spaced Repetition Scheduler (v6, 21 parameters) |
| go-git | Pure Go git library for version control (no git CLI needed) |
| Graph explorer | Cytoscape.js interactive graph in explore mode |
| Item | Node marked `@card`/`@cloze`, scheduled by FSRS |
| Knowledge Architect | AI Socratic tutor (Feynman Technique) |
| Learning state export | JSON export of FSRS review history for multi-device sync |
| Leech | Card with 5+ lapses |
| Leads to (LT) | Link type from `_sequence_` mode |
| Neural review | Typed spreading activation over the knowledge graph |
| Orbit minimap | Radius-1 view in top band (from BadgerDB) |
| Priority queue | Unified 0–100% queue for topics, extracts, items |
| Read view | Rendered presentation of N4L file with inline media |
| Reminder | ALL CAPS line = open loop / knowledge debt |
| Retrievability (R) | FSRS current recall probability |
| Save file | Ctrl+S: write .n4l to disk + sync SQLite |
| Sequence card | Card testing ordered recall in `_sequence_` chains |
| SSTorytime-KV | BadgerDB port of SSTorytime (embedded, serverless) |
| Stability (S) | FSRS storage strength of memory |
| Strategy | Neural review direction: causal, decomposition, property, analogy, free |
| Sync | Push git commits to configured remote (GitHub/GitLab) |
| Topic | Chapter or context scheduled for periodic curation |
| Version history | Git-backed timeline of knowledge evolution, viewable in history panel |

---

*— End of specification —*
