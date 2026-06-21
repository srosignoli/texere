# Incremental Reading — Phase 2c (PDF & Web Adapters) Design

**Date:** 2026-06-21
**Status:** Design approved, pending spec review
**Parent strategy:** `docs/superpowers/specs/2026-06-20-incremental-reading-design.md` (Phase 2, slice 2c)
**Builds on:** Phase 1 (008) + 2a read & extract (009) + 2b process & unify (010).

## 1. Context & slice

Phase 2's third slice completes "read anything" by adding the two remaining source media behind the existing reading-element/extract model: **PDF** (read in a visual pdf.js pane, page anchors) and **web articles** (imported by URL, rich sanitized-HTML reader, images preserved locally). The explorer "Study this" bridge and the Process "Split" action are **deferred to a later follow-up** (not in 2c).

The reading-element + extract + `(from)`-anchor machinery from 2a/2b is unchanged; 2c only adds new per-medium source panes and import paths.

## 2. Decisions (from brainstorming)

| # | Decision |
|---|---|
| 1 | **2c scope = PDF + web adapters.** Explorer "Study this" bridge and Split → later follow-up. |
| 2 | **PDF reader = visual pdf.js** (canvas + selectable text layer), page-based `(from)` anchors (`page=N`), resume-at-page. PDF bytes are served to the frontend; PDF bypasses backend `source.Projection`. |
| 3 | **Web = rich article, images preserved.** Import URL → fetch → Readability extract → **sanitize** → **download images and inline as `data:` URIs** → save a self-contained `sources/<slug>.html` → record the **original URL**. Read in a rich sanitized-HTML reader; `char=N` anchors; "Open original" reopens the URL. |
| 4 | **Medium dispatch via a `medium` column** (schema v4) on `reading_elements`; `ReadingView` is refactored into a shell with **pluggable source panes** (Text / Pdf / Web). |
| 5 | **Pinned deps (pure Go):** `github.com/go-shiori/go-readability` + `github.com/microcosm-cc/bluemonday`. |

## 3. Scope & non-goals

**In scope (2c):** PDF import + visual reader (page anchors, resume); web-URL import (fetch + Readability + sanitize + image-inlining + save + original-URL record) + rich-HTML reader (char anchors, "Open original"); schema v4 (`medium` + `source_url`); the `ReadingView` pluggable-pane refactor; Sources-panel import buttons for PDF and web.

**Out of scope:** the explorer "Study this" bridge; Split; audio/video (Phase 3); rich PDF annotation/highlight persistence beyond extract; re-fetch/refresh of a saved web article; the exact 20/15/65 queue mix + priority/optimizer/stats (Phase 3).

## 4. Data & anchor model

Per-medium dispatch (medium derived at import, stored on the reading element):

| Medium | `sources/` file | reader | `(from)` anchor & `# @progress` |
|---|---|---|---|
| text/md (2a) | the file | `TextSourcePane` (plain text) | `char=N` (rune offset) |
| **web (2c)** | self-contained sanitized article HTML (images inlined as `data:` URIs) | `WebSourcePane` (rich HTML) | `char=N` into the article's rendered text |
| **pdf (2c)** | the PDF | `PdfSourcePane` (pdf.js canvas + text layer) | `page=N` |

- The `(from)` anchor string keeps the 2a form `"<refPath> <unit>=N"` — `char=N` for text/web, `page=N` for PDF. The extract/`# @progress`/scheduling machinery treats the anchor as an opaque string, so no change there.
- **Both web and PDF bypass backend `source.Projection`**: the frontend pane owns rendering, text, selection offsets, and (for PDF) page count, reporting them to the backend. `source.Projection` stays the text/md path only.
- The reading element gains `medium` ('text'|'web'|'pdf') and `source_url` (the original URL for web; "" otherwise).

## 5. Flows

### 5.1 PDF

1. **Import** (`ImportPdfSource()`): file dialog (`.pdf`) → copy into `sources/` (suffix on collision) → `CreateReadingNotes` scaffold → register a reading element (`medium='pdf'`, `# @progress page=1`).
2. **Read** (`PdfSourcePane`): `GetSourceBytes(refPath)` → base64 → pdf.js `getDocument`; render the current page to a canvas with a selectable text layer; page nav (prev/next/jump); resume at `# @progress page=N`; on first load report total pages → backend sets `proj_len`. Select text → `extract({text, "<ref> page=<N>"})`. Leaving → `# @progress page=N` + reschedule.

### 5.2 Web

1. **Import** (`ImportWebSource(url)`): Go `net/http` GET → `go-readability` article (title + content HTML) → **bluemonday sanitize** (strip `<script>`, inline handlers, `<iframe>`, external resource loads) → for each remaining `<img src>`, download the image and rewrite to a `data:<mime>;base64,…` URI (cap per-image bytes; oversized → drop the `<img>`, keep the article) → save the self-contained HTML at `sources/<slug>.html` → `CreateReadingNotes` scaffold (`# @source "<title>"`, original URL recorded) → register a reading element (`medium='web'`, `source_url=<url>`).
2. **Read** (`WebSourcePane`): render the saved sanitized HTML in a scrollable container (images show inline); a DOM-range selection maps to a rune char offset into the container's text content → `extract({text, "<ref> char=<N>"})`; resume scrolls to `# @progress char=N`; an **"Open original"** button opens `source_url` via `runtime.BrowserOpenURL`.

## 6. Architecture / components

**Backend:**

| Unit | Responsibility | New/changed |
|---|---|---|
| `internal/source/web.go` (+`_test.go`) | `FetchAndExtract(url) (WebArticle, error)` → `{Title, HTML (sanitized, images inlined), URL}`. Fetch + go-readability + bluemonday + image-inline. | **new** |
| `internal/learning/` | Schema **v4**: `ALTER TABLE reading_elements ADD COLUMN medium TEXT NOT NULL DEFAULT 'text'` + `ADD COLUMN source_url TEXT NOT NULL DEFAULT ''`; carry `Medium`/`SourceURL` on `ReadingElement` + CRUD/scan. | extend |
| `reading_methods.go` / `study_methods.go` | `ImportPdfSource()`, `ImportWebSource(url)`, `GetSourceBytes(refPath) → string` (base64), set `proj_len`/page count, `OpenSourceURL(id)` (BrowserOpenURL). `ReadingView`/list views include `medium`/`sourceUrl`. | extend |

**Frontend:**

| Unit | Responsibility | New/changed |
|---|---|---|
| `ReadingView.svelte` | Shell: pick the source pane by `reading.medium`; own the shared notes editor + Extract/Later/Done wiring; the pane emits `extract({text, anchor})` and `position({anchor})`. | refactor |
| `TextSourcePane.svelte` | The 2a inline text rendering + char-offset selection, extracted from ReadingView. | **new** (refactor) |
| `PdfSourcePane.svelte` | pdf.js render (canvas + text layer), page nav, resume page, total-page report; selection → `{text, page=N}`. | **new** |
| `WebSourcePane.svelte` | Render sanitized article HTML; DOM-range selection → `{text, char=N}`; "Open original". | **new** |
| `SourcesPanel.svelte` | Add **Import PDF** + **Import Web (URL prompt)** buttons. | extend |

**Reuse:** the notes editor, Extract/`# @progress`/reschedule logic, `CreateReadingNotes`, the reading-element scheduling, and the StudySurface Read focus are all unchanged — 2c only swaps the left source pane and adds two import paths.

## 7. `learning.db` schema v4

Stepwise migrate gains `case 3 → ddlV4`:

```sql
ALTER TABLE reading_elements ADD COLUMN medium TEXT NOT NULL DEFAULT 'text';
ALTER TABLE reading_elements ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
```

Additive: existing rows become `medium='text'`, `source_url=''`; no data loss; idempotent re-open. `schema_meta.version` advances 3→4.

## 8. Security (hard requirement)

The web path fetches and renders third-party markup — the primary risk surface in 2c.
- **Sanitize before save:** bluemonday with a strict policy — allow article structural/text tags + `<img>` (data-URI only after inlining); strip `<script>`, all `on*` handlers, `<iframe>`/`<object>`/`<embed>`, `<style>`/`<link>`, and any non-`data:`/`https:`-image external loads.
- **Inline images only** (`data:` URIs); the saved/rendered HTML must reference **no remote resources**, so reading it never makes a network request.
- **Render inert:** the WebSourcePane renders the sanitized HTML into a normal element (not an iframe with scripts); since scripts/handlers are stripped, no execution occurs. The fetch itself uses a timeout + a max response size.

## 9. Error handling / edge cases

- **Web fetch fails / non-2xx / timeout:** surface an import-error toast; create **no** source/notes/element (atomic — nothing partial left behind).
- **Readability yields little:** save whatever article content was found (possibly short); not an error.
- **Oversized / un-fetchable images:** drop that `<img>` (keep the article); cap total inlined bytes.
- **PDF corrupt / password-protected:** the PdfSourcePane shows an error state; the element/notes remain.
- **Resume page/char out of range** (source changed): clamp to range.
- **Selection spanning PDF pages:** anchor to the selection's start page.
- **Schema v4 on a v3 DB:** additive; no data loss; idempotent.
- **`source_url` portability:** stored on the reading element (local DB) and the original URL written into the notes `# @source` so the reference survives.

## 10. Testing

- **Go unit:** `FetchAndExtract` against a local `httptest.Server` serving a fixture HTML (with a `<script>` and an `<img>`) — assert the title is extracted, the `<script>` is stripped, the `<img>` is inlined as a `data:` URI (no remote `src` remains), and the returned `URL` is the original; schema-v4 migrate-from-v3 + `medium`/`source_url` CRUD; `ImportPdfSource`/`ImportWebSource` register the right `medium`; `GetSourceBytes` round-trips a file as base64.
- **Manual quickstart matrix:** import a PDF → visual reader renders pages → select text → an extract with a `page=N` anchor lands + `# @progress` updates → reopen resumes the page; import a web URL → the rich article renders **with its image** → select → an extract with a `char=N` anchor → "Open original" opens the URL; confirm the saved HTML contains no `<script>` and no remote `src`.

## 11. Deferred (recorded)

- Explorer "Study this" bridge; Split (a later follow-up slice).
- Audio/video; web article re-fetch/refresh; PDF highlight persistence; the Phase-3 items (queue mix, priority transparency, FSRS optimizer, stats).
- **Cross-slice review checkpoint:** per strategy §8, after 2c run an **opus a+b+c coherence review** scoped to `main..HEAD` before merging the Phase-1→2 stack (verify the rune-offset `char=` contract across panes, the `medium` dispatch, and the promote/discovery round-trip end-to-end).
