---
description: "Task list for 001-project-scaffold"
---

# Tasks: Project Scaffold

**Input**: Design documents from `/docs/specs/001-project-scaffold/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/build-contract.md ✅, quickstart.md ✅

**Tests**: No test tasks generated (not explicitly requested in spec). One smoke test (app_test.go) is included as part of US2 quality-gate infrastructure.

**Organization**: Tasks grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[USn]**: Maps to user story from spec.md
- All file paths are relative to repository root

---

## Phase 1: Setup (Project Initialization)

**Purpose**: Create the Wails v2 + Svelte project skeleton. Must complete before any user story work begins.

- [x] T001 Initialize Wails v2 project with Svelte template by running `wails init -n texere -t svelte` at repository root (creates main.go, app.go, wails.json, go.mod, go.sum, frontend/, build/)
- [x] T002 Update go.mod to set module path `github.com/<owner>/texere` and minimum version directive `go 1.21` at go.mod

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build interface that ALL user stories depend on. MUST be complete before Phase 3.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T003 Create Makefile at Makefile with targets: `build` (Linux amd64 via `wails build`), `build-windows` (Windows amd64 via `CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 wails build -platform windows/amd64`), `test` (`go test ./...`), `lint` (`golangci-lint run ./...`), `dev` (`wails dev`), `clean` (removes build/bin/ and frontend/dist/)

**Checkpoint**: Makefile exists and `make --dry-run build` shows the expected command — user story implementation can now begin.

---

## Phase 3: User Story 1 — Developer Runs the Application (Priority: P1) 🎯 MVP

**Goal**: A developer clones the repo, runs `make build`, and launches a binary that opens a Texere placeholder window.

**Independent Test**: Run `make build` → run `./build/bin/texere` → window opens with "Texere" title visible. Run `make build-windows` → `build/bin/texere.exe` exists and `build/bin/texere` (Linux) is untouched.

### Implementation for User Story 1

- [x] T004 [P] [US1] Update app.go to define `App` struct with Wails lifecycle methods `startup(ctx context.Context)`, `domReady(ctx context.Context)`, `beforeClose(ctx context.Context) bool`, and `shutdown()` — stubs only, no business logic at app.go
- [x] T005 [P] [US1] Update frontend/src/App.svelte to display a centred "Texere" heading and a subtitle "Your knowledge workbench" as a static placeholder (no dynamic data) at frontend/src/App.svelte
- [x] T006 [US1] Run `make build` — produces build/bin/texere (7.3MB, no missing libs); uses -tags webkit2_41 for webkit2gtk-4.1 compatibility and verify `build/bin/texere` is produced, binary is self-contained (no shared lib errors via `ldd build/bin/texere`), and SC-001 build time is under 5 minutes (depends on T004, T005)
- [x] T007 [US1] Run `make build-windows` — produces build/bin/texere.exe (9.6MB); Linux binary untouched and verify `build/bin/texere.exe` is produced and `build/bin/texere` (Linux binary) is not overwritten (depends on T006)

**Checkpoint**: User Story 1 complete — `make build` and `make build-windows` both succeed; binary opens Texere window.

---

## Phase 4: User Story 2 — Developer Runs Tests and Linting (Priority: P2)

**Goal**: A developer runs `make test` and `make lint` from the repository root; both pass with zero ambiguous failures on the unmodified scaffold.

**Independent Test**: Run `make test` → all tests pass, count reported. Run `make lint` → zero violations. Introduce a deliberate formatting error → `make lint` reports file, line, and rule name.

### Implementation for User Story 2

- [x] T008 [P] [US2] Create app_test.go with `TestAppCreation` that instantiates `App{}` and asserts the struct is non-nil, confirming the App package compiles and the zero value is valid at app_test.go
- [x] T009 [P] [US2] Create .golangci.yml enabling linters: `govet`, `errcheck`, `staticcheck`, `gofmt`, `goimports`; exclude directories `frontend/` and `build/` from analysis at .golangci.yml
- [x] T010 [US2] Run `make test` and verify all tests pass with pass count reported in output and exit code 0; confirm completion in under 60 seconds (SC-003, FR-004) (depends on T008)
- [x] T011 [US2] Run `make lint` and verify zero violations on unmodified scaffold (SC-004, FR-005); then introduce a deliberate `gofmt` violation and confirm lint reports file path, line number, and rule name (depends on T009)

**Checkpoint**: User Story 2 complete — `make test` and `make lint` both pass cleanly.

---

## Phase 5: User Story 3 — Developer Navigates the Project Structure (Priority: P3)

**Goal**: A developer reads docs/quickstart.md and correctly identifies where to add code in each of the three constitutional layers without additional help.

**Independent Test**: Read docs/quickstart.md → locate the "Project Structure" and "Where to add new features" sections → identify correct paths for content layer, graph layer, learning layer, and UI layer additions.

### Implementation for User Story 3

- [x] T012 [P] [US3] Create internal/content/doc.go with package declaration `package content` and doc comment: "Package content implements Layer I of the Texere architecture: reading, writing, and parsing .n4l knowledge files. Only this package may read or write .n4l files." at internal/content/doc.go
- [x] T013 [P] [US3] Create internal/graph/doc.go with package declaration `package graph` and doc comment: "Package graph implements Layer II of the Texere architecture: knowledge graph storage and querying via BadgerDB. This is a derived cache — always rebuildable from the content layer." at internal/graph/doc.go
- [x] T014 [P] [US3] Create internal/learning/doc.go with package declaration `package learning` and doc comment: "Package learning implements Layer III of the Texere architecture: FSRS spaced-repetition scheduling state via SQLite. This is a derived store — losing it causes history loss but not knowledge loss." at internal/learning/doc.go
- [x] T015 [US3] Create docs/quickstart.md from docs/specs/001-project-scaffold/quickstart.md: copy content and adjust any spec-relative paths to repo-root-relative paths; ensure the "Project Structure" section reflects the actual generated tree and the "Where to add new features" table lists all four layers at docs/quickstart.md (depends on T012, T013, T014)

**Checkpoint**: User Story 3 complete — docs/quickstart.md exists; internal/ tree has all three layer stubs with constitutional doc comments.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: CI automation and agent context finalization that spans all user stories.

- [x] T016 [P] Create .github/workflows/ci.yml with two jobs: `build-and-test` (runner: ubuntu-latest; steps: checkout, setup-go@v5 with go-version 1.21, install wails via `go install github.com/wailsapp/wails/v2/cmd/wails@latest`, `make build`, `make test`, `make lint`) and `build-windows` (runner: ubuntu-latest; steps: checkout, setup-go@v5, `sudo apt-get install -y gcc-mingw-w64-x86-64`, install wails, `make build-windows`); trigger on push and pull_request to main at .github/workflows/ci.yml
- [ ] T017 Push feature branch to GitHub ⚠️ PENDING: requires remote push (needs user confirmation) and verify both CI jobs complete with exit code 0 within 10 minutes (SC-006, FR-009) (depends on T016)
- [ ] T018 [P] Verify docs/quickstart.md instructions ⚠️ BLOCKED: full end-to-end validation requires T006 (binary build) to complete first are accurate by following the Prerequisites and "Clone and Build" sections on the current machine and confirming `./build/bin/texere` launches successfully (end-to-end validation of SC-005)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 completion — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Phase 2 — no dependency on US2 or US3
- **User Story 2 (Phase 4)**: Depends on Phase 2 — no dependency on US1 (but T008 benefits from T001 completing first)
- **User Story 3 (Phase 5)**: Depends on Phase 2 — no dependency on US1 or US2
- **Polish (Phase 6)**: T016/T018 can start after Phase 3+4 complete; T017 requires T016

### User Story Dependencies

- **US1 (P1)**: Can start after Phase 2 — T004 and T005 are parallel within the story
- **US2 (P2)**: Can start after Phase 2 — T008 and T009 are parallel within the story
- **US3 (P3)**: Can start after Phase 2 — T012, T013, T014 are all parallel

### Within Each User Story

- Parallel tasks within a story (marked [P]) can start simultaneously
- Verification tasks (T006, T007, T010, T011) depend on their respective implementation tasks
- T015 depends on T012–T014 (internal/ stubs must exist before doc is validated against actual paths)

### Parallel Opportunities

```bash
# After Phase 2 is complete, all three user stories can start in parallel:

# US1 parallel block:
Task T004: Update app.go lifecycle stubs
Task T005: Update frontend/src/App.svelte placeholder

# US2 parallel block (simultaneously with US1):
Task T008: Create app_test.go smoke test
Task T009: Create .golangci.yml

# US3 parallel block (simultaneously with US1 and US2):
Task T012: Create internal/content/doc.go
Task T013: Create internal/graph/doc.go
Task T014: Create internal/learning/doc.go
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001–T002)
2. Complete Phase 2: Foundational (T003) — CRITICAL, blocks everything
3. Complete Phase 3: User Story 1 (T004–T007)
4. **STOP and VALIDATE**: `make build` → launch binary → confirm "Texere" window opens
5. `make build-windows` → confirm .exe produced

### Incremental Delivery

1. Phase 1 + Phase 2 → Build interface ready
2. US1 → Binary builds and launches (MVP!)
3. US2 → Tests and lint pass cleanly
4. US3 → Project structure documented
5. Polish → CI green, quickstart validated

---

## Notes

- [P] tasks have no file conflicts and no incomplete dependencies — safe to run in parallel
- T001 (`wails init`) may modify files already present; run from a clean working tree
- T002 must follow T001 since wails init creates go.mod
- T006 and T007 are validation steps — their pass/fail directly confirms US1 acceptance criteria
- T010 and T011 are validation steps — their pass/fail directly confirms US2 acceptance criteria
- `<owner>` in T002 should be replaced with the actual GitHub organisation/user before committing go.mod
- Avoid: implementing any real N4L parsing, graph queries, or learning logic — this is scaffold only
