<!--
SYNC IMPACT REPORT
==================
Version change: [unversioned template] → 1.0.0
Modified principles: N/A (initial ratification — all principles new)
Added sections:
  - Core Principles (5 principles)
  - Performance Standards
  - Development Workflow
  - Governance
Removed sections: N/A
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ (Constitution Check gates align with principles below)
  - .specify/templates/spec-template.md ✅ (no structural changes required; requirements format retained)
  - .specify/templates/tasks-template.md ✅ (task categories and observability/versioning types consistent)
  - .specify/templates/constitution-template.md ✅ (source template; no changes required)
Follow-up TODOs:
  - None — all fields resolved from spec and project context.
-->

# Texere Constitution

## Core Principles

### I. Source-First & Full N4L Fidelity

The `.n4l` plain-text file is the single source of truth for all knowledge content.
Every feature MUST preserve round-trip fidelity: a file read in and written back out
MUST be byte-identical unless the user explicitly edits it. The full N4L syntax surface
MUST be supported: context operators (`::`, `+::`, `-::`), sequence mode (`_sequence_`),
nodes/arrows/ditto chains, labels, references (`@`, `$PREV`), annotation symbols, and
ALL CAPS reminders. No proprietary binary knowledge format is permitted as a primary store.

**Rationale**: Plain text ensures longevity, portability, and user ownership. Users can
open, edit, and version-control their notes with any tool independent of this application.

### II. Self-Contained Binary

The workbench MUST ship as a single Go binary with zero external runtime dependencies.
No database server, no Docker container, no git CLI, no credentials, and no network
connectivity are required for core functionality. All storage engines MUST be pure Go
(no CGo): `modernc.org/sqlite` for learning state, BadgerDB for the knowledge graph,
`go-git` for version control. Cross-compilation to Windows and Linux MUST be supported
without additional tooling.

**Rationale**: A single-binary, zero-dependency application eliminates installation
friction, maximises portability, and guarantees the application works offline and on
machines without administrator privileges.

### III. Separation of Concerns

The three data layers MUST remain strictly decoupled:

- **Content** — `.n4l` plain-text files (canonical, user-owned)
- **Knowledge graph** — SSTorytime-KV (BadgerDB); derived, always rebuildable from source
- **Learning state** — SQLite (`learning.db`); derived, lossy rebuild acceptable

No feature is permitted to blur these boundaries. The knowledge graph and learning state
are caches derived from source; losing them MUST never result in permanent data loss.
Features MUST NOT store canonical knowledge exclusively in BadgerDB or SQLite.

**Rationale**: Clear layer boundaries enable independent evolution, safe migration, and
correct mental models for both developers and users.

### IV. Pleasure-Driven Learning

Every study-facing feature MUST prioritise learner agency, variety, and visible progress.
Study sessions SHOULD interleave topics, extracts, and items. Users MUST be able to
override any automatic scheduling decision. All FSRS state transitions MUST be visible.
Features that introduce friction without measurable learning benefit MUST NOT be merged.
Neural review MUST offer creative knowledge exploration, not rote repetition.

**Rationale**: Sustained knowledge work requires motivation. The system earns daily use
only by making study feel rewarding rather than punitive.

### V. Transparency & Portability

Every automatic decision the system makes (priority score, FSRS grade, reminder boost,
git auto-commit message) MUST be surfaced to the user in a readable form and MUST be
overridable. The entire knowledge collection MUST be a plain folder the user can zip,
move, or open in any git client. Remote sync (GitHub/GitLab) MUST be opt-in, never
silent, and never required.

**Rationale**: Users must trust the system. Trust requires transparency. Portability
ensures the system never holds user data hostage.

## Performance Standards

All interactive operations MUST meet the following latency budgets measured on a modern
consumer laptop with a 10 000-line collection:

| Operation | Budget | Notes |
|---|---|---|
| N4L parse + context replay | < 50 ms | Single-pass |
| SSTconfig registry build | < 10 ms | Read once at startup |
| Build graph (Ctrl+G) | 100–500 ms | BadgerDB batch write |
| Orbit query (radius 1) | < 1 ms | BadgerDB key lookup |
| Graph explorer frame 0 (skeleton) | ≤ 16 ms | Ground plane + territory polygons + camera frame; runs synchronously on the IPC return so the user sees the empty isometric world immediately. |
| Graph explorer full settle | ≤ 2 s | All node + edge meshes built and fade-in tweens complete; staggered by `distanceFromAnchor` to keep each frame ≤ 8 ms during construction. |
| Study queue build | < 100 ms | SQLite indexed query |
| Neural activation per hop | < 50 ms | BFS with weighted edges |
| Read view render | < 200 ms | HTML from parsed AST |
| FSRS optimizer | < 30 s / 20 K reviews | Background thread |

Performance regressions against these budgets constitute a constitution violation and
MUST be justified and approved before merge.

## Development Workflow

**Technology stack** — Changes to the canonical technology choices below MUST be
documented as a constitution amendment with migration rationale:

| Layer | Technology |
|---|---|
| Application framework | Wails v2 (Go + OS webview) |
| Source editor | CodeMirror 6 |
| Graph visualization | Three.js (2.5D isometric WebGL canvas) |
| Knowledge graph | SSTorytime-KV (BadgerDB, pure Go) |
| Learning state | SQLite (`modernc.org/sqlite`, pure Go) |
| Version control | `go-git` (pure Go) |
| Frontend framework | Svelte (preferred) |

**Dependency discipline** — New dependencies MUST be pure Go (no CGo) unless a
compelling, documented exception is approved. All exceptions MUST appear in
`Complexity Tracking` sections of the relevant plan.

**Testing** — All public APIs and scheduling algorithms MUST have unit tests. N4L
parser changes MUST include round-trip fidelity tests. Performance-sensitive paths
MUST include benchmark tests that can be run in CI.

**Versioning** — The application follows Semantic Versioning. Breaking changes to
`.n4l` syntax parsing, the learning.db schema, or the BadgerDB graph schema constitute
MAJOR version bumps and MUST include a migration path for existing user collections.

## Governance

This constitution supersedes all other development guidelines. Amendments require:

1. A written proposal documenting the motivation and affected principles.
2. Version bump per semantic rules (MAJOR/MINOR/PATCH as defined in constitution header).
3. `Last Amended` date updated to the amendment date.
4. A migration plan for any breaking changes affecting user data.
5. Updated `Sync Impact Report` prepended to this file.

All plan reviews (via `/speckit-plan`) MUST include a **Constitution Check** gate that
validates the proposed implementation against all five core principles and the
performance standards before Phase 0 research begins.

All PRs MUST be reviewed for compliance. Complexity that violates these principles MUST
be justified in the plan's `Complexity Tracking` table.

For runtime development guidance, consult `.specify/integrations/claude.manifest.json`
and the skills in `.claude/skills/`.

**Version**: 1.0.0 | **Ratified**: 2026-04-06 | **Last Amended**: 2026-04-06
