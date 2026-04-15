# Research: Editor Core — Phase 0 Findings

**Branch**: `003-editor-core` | **Date**: 2026-04-09

All NEEDS CLARIFICATION items resolved. No items remain open.

---

## 1. CodeMirror 6 + Svelte 3 Integration

**Decision**: Mount a `EditorView` instance imperatively inside a Svelte `onMount` callback. Destroy it in `onDestroy`. Pass updated content programmatically via `view.dispatch(view.state.update({changes: ...}))`.

**Rationale**: CodeMirror 6 is framework-agnostic and manages its own DOM node. Svelte's reactive bindings are not used for the editor DOM — only for the container element reference. This is the idiomatic CM6 pattern regardless of the surrounding framework.

**Alternatives considered**:
- `@replit/codemirror-svelte` wrapper: adds a dependency for no real gain; the manual `onMount` pattern is 20 lines.
- Svelte 4 / SvelteKit: would require upgrading the scaffold; deferred until a future feature that warrants it.

**Key implementation notes**:
- Use a `bind:this={editorContainer}` div as the parent element.
- Create `EditorView` with `parent: editorContainer` inside `onMount`.
- When `selectedNode` changes (from store), dispatch a scroll-into-view effect rather than recreating the editor.
- External content changes (file reload): replace the full document via a single `dispatch` with a `changes` that replaces `[0, state.doc.length]`.

---

## 2. N4L Syntax Highlighting in CodeMirror 6

**Decision**: Use `StreamLanguage.define()` from `@codemirror/language` with a hand-written `StreamParser<State>`.

**Rationale**: N4L is strictly line-oriented — every syntactic element is determinable from the first characters of a line (with the exception of inline arrows). A `StreamParser` processes one line at a time and is significantly simpler to write, test, and maintain than a Lezer grammar. A full Lezer grammar would be warranted if we needed incremental re-parsing of nested structures; N4L has none.

**Alternatives considered**:
- Full Lezer grammar: overkill; would require a separate build step for the grammar compiler.
- Raw `Decoration`s without a language extension: loses the `syntaxHighlighting` integration and requires manual range tracking.

**Token types to define** (mapping to CM6 highlight tags):

| N4L pattern | CM6 tag | Visual role |
|---|---|---|
| `- title` line | `tags.heading` | Chapter header |
| `:: ...` / `+:: ...` / `-:: ...` | `tags.keyword` | Context operator |
| `(code) target` arrow inline | `tags.operatorKeyword` + `tags.string` | Arrow code + target |
| `"..."` ditto line | `tags.meta` | Ditto continuation |
| `# @card` / `# @study` / `# @cloze` / `# @end` | `tags.annotation` | Study markers |
| `# @id` / `# @source` / `# @ref` / `# @progress` | `tags.comment` (muted) | Provenance markers |
| ALL CAPS line (≥ 3 words, no lowercase) | `tags.invalid` (amber override) | Reminder |
| `@label` anchor | `tags.labelName` | Anchor |
| `# regular comment` | `tags.lineComment` | Inline comment |

---

## 3. Arrow Autocomplete in CodeMirror 6

**Decision**: Register a custom `CompletionSource` via the `autocompletion` extension. The source activates when the cursor is on a line that matches the `(` prefix of an arrow code, and returns completions drawn from the `ArrowDef[]` fetched from Go at startup.

**Rationale**: `@codemirror/autocomplete` provides an `autocompletion` extension that accepts an array of `CompletionSource` functions. Each source receives a `CompletionContext` and returns a `CompletionResult` or `null`. This is sufficient for our use case.

**Completion trigger**: match regex `\(\w*$` (open paren followed by zero-or-more word chars ending at cursor). Complete with `{label: def.Code, detail: def.LongName, info: def.MetaType}`. Group by `MetaType` using the `section` field.

**Alternatives considered**:
- Hard-coding arrow codes: couples frontend to SSTconfig content; rejected.
- Fetching completions on each keystroke from Go: unnecessary round-trip; the registry is small and static at runtime.

**Implementation note**: Call `GetArrowRegistry()` once during `App.svelte` mount and store the result in `arrowDefsStore`. The `CompletionSource` closure captures this store value.

---

## 4. Wails v2 EventsEmit Pattern

**Decision**: Use `runtime.EventsEmit(ctx, eventName, data)` on the Go side and `window.runtime.EventsOn(eventName, callback)` (imported from `wailsjs/runtime/runtime`) on the Svelte side.

**Rationale**: Wails v2 provides a bidirectional event bus. Go emits via `runtime.EventsEmit`; the frontend subscribes via `EventsOn`. The frontend receives events as JavaScript callbacks with the payload deserialized from JSON automatically.

**Events used by this feature**:

| Event name | Direction | Payload | Trigger |
|---|---|---|---|
| `collection:change` | Go → Svelte | `CollectionEvent` | `consumeWatchEvents` loop in `app.go` |
| `file:modified-externally` | Go → Svelte | `{ path: string }` | `consumeWatchEvents` when `EventModified` is for the currently open file |

**Implementation note**: `consumeWatchEvents` in `app.go` already has a `TODO` comment for `runtime.EventsEmit`. This is the only Go-side change needed for event wiring. The Svelte side subscribes in `onMount` of `App.svelte` and unsubscribes via the returned cancellation function in `onDestroy`.

---

## 5. Context Tag Computation — Frontend vs. Backend

**Decision**: Compute context chips and node filtering entirely in the frontend using `Node.EffectiveCtx` fields already present in `ParsedFile`.

**Rationale**: `ParseFile` already stores the computed `EffectiveCtx` snapshot on every `Node` (see `types.go:Node.EffectiveCtx`). Column 2 chips = union of all unique tags across nodes in the selected chapter. Column 3 filter = nodes where `EffectiveCtx` is a superset of all active chips. No additional Go computation is needed.

**Alternatives considered**:
- A dedicated Go method to compute context sets per chapter: unnecessary given `EffectiveCtx` is already computed during parse.

---

## 6. Save Flow — Atomic Write + ID Injection

**Decision**: The `SaveFile` Go method writes the edited content atomically, parses the saved file, calls `InjectMissingIDs`, and returns the final file content (post-injection) along with the injection list.

**Rationale**: `writeLines` in `writeback.go` already implements temp-file-then-rename atomics. `InjectMissingIDs` requires a `*ParsedFile` obtained from `ParseFile`. Two sequential atomic writes occur when IDs are injected (first write: user content; second write: + injected IDs). The watcher will fire `EventModified` twice in this case; `consumeWatchEvents` must suppress these internally-triggered events (see implementation note below).

**Internally-triggered event suppression**: Track the path of the file currently being saved in `app.go` (`savingPath string`). In `consumeWatchEvents`, skip emitting `file:modified-externally` when `event.Path == a.savingPath`.

**Final content return**: After injection, `os.ReadFile(path)` returns the definitive on-disk content. The frontend replaces the editor document with this content so the `# @id` lines inserted by injection become visible.

---

## 7. External File Modification Detection

**Decision**: Reuse `CollectionWatcher` (already watching the collection directory). Filter `EventModified` events in `consumeWatchEvents`; emit `file:modified-externally` only when the modified path matches the currently open file and the modification was not triggered by our own save.

**Rationale**: `CollectionWatcher` (wrapping `fsnotify`) already emits `EventModified` for any `.n4l` file change in the directory. No second watcher is needed.

---

## 8. Read Tab — Rendering Strategy

**Decision**: Pure Svelte templating from the `ParsedFile` AST. No additional rendering library.

**Rationale**: `ParsedFile` provides all necessary structure (chapters → nodes → arrows, reminders, sequence flags). Each `Node` becomes a card component. Arrows are rendered as labeled `<dl>` entries. Media references (images, audio) are rendered as `<img>` / `<audio>` tags with `src` pointing at the file path relative to the collection directory. PDF thumbnails use a `<canvas>` with `pdf.js` (added as a dev dependency if needed, bundled by Vite).

**Sequence connector**: CSS `::before` / `::after` pseudo-elements on `.sequence-node` cards, using a vertical line motif.

**Alternatives considered**:
- A Markdown-to-HTML pipeline: N4L is not Markdown; mapping N4L structure to Markdown would lose semantics.
- A dedicated rendering library (e.g., Lit): unnecessary overhead for a Svelte project.
