# Specification Quality Checklist: Replace text matching with Bleve full-text search engine

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-04-24
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

**Notes on Content Quality**:

- The spec deliberately *does* reference Bleve, `bleve/v2`, BadgerDB, file paths, and function/line references. This is a principled exception: the feature's stated purpose is "replace X with Bleve", and the pre-spec research section is an explicit requirement from the user who asked for a concrete description of the current text-match code before committing to the work. Stripping these references would make the spec less actionable and contradict the user's instructions. Flagging so reviewers can confirm the exception is appropriate for this feature.

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

**Notes on Requirement Completeness**:

- SC-008 (≤ 200 ms Build-graph overhead) and SC-009 (≤ 50 MB index for 5 000 nodes) are quantitative but will need a baseline measurement during planning; both are drawn directly from the user's stated success criteria.
- Success criteria mention Bleve-specific query primitives in acceptance examples (`"fish soup"`, `flo:*`, `!A!`, etc.). These describe **user-facing query syntax**, not implementation — they are the input grammar the user operates. The underlying engine is named in SC headers only where omitting it would make the criterion untestable.

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- This feature spans two code owners: Texere (`internal/search/search.go`) and SSTorytime-KV (`kv_badger.go`, `store_paths.go`, `store_indexes.go`). The pre-spec research section documents this scope split so the plan phase can decompose it correctly.
- FR-022 (missing-index error path) and the "Collection has a `sst_data/` but no `bleve/`" edge case cover the migration case for existing users without forcing a data migration.
- No `/speckit.clarify` round is blocking; the user's original description was complete enough to spec directly. Remaining v1 decisions (exact Bleve analyzer, exact wire format for the new SSTorytime-KV API) belong in the plan phase.
