# Feature Specification: Project Scaffold

**Feature Branch**: `001-project-scaffold`
**Created**: 2026-04-06
**Status**: Draft
**Input**: User description: "000-project-scaffold"

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Developer Runs the Application from a Fresh Clone (Priority: P1)

A new contributor clones the repository, follows the getting-started instructions,
and has Texere running on their machine without any manual environment
configuration, external service setup, or system-level dependency installation beyond
the standard build toolchain.

**Why this priority**: This is the entry point for all subsequent development. If
onboarding is broken, no other feature can be built or tested. It also validates
that the self-contained binary principle (Constitution §II) is met end-to-end from
the very first commit.

**Independent Test**: A developer with a clean machine (only the build toolchain
installed) can clone the repo, run a single build command, launch the resulting
binary, and see the application window open — no additional setup required.

**Acceptance Scenarios**:

1. **Given** a machine with only the build toolchain installed, **When** the developer
   runs the standard build command from the repository root, **Then** a single
   distributable binary is produced with no errors and no external dependencies.
2. **Given** the produced binary, **When** the developer launches it without any
   arguments or configuration files, **Then** the application opens and displays a
   static placeholder window with the "Texere" application name visible.
3. **Given** a Linux development machine, **When** the developer runs the
   cross-compilation command targeting Windows (amd64), **Then** a runnable Windows
   binary is produced without additional tooling or OS-level dependencies.

---

### User Story 2 — Developer Runs Tests and Linting (Priority: P2)

A developer makes a code change and runs the project's test and lint commands
from the repository root to verify correctness and style compliance before
committing.

**Why this priority**: Consistent, runnable quality gates from day one prevent
quality debt accumulation and enable CI integration. Required before any feature
implementation begins.

**Independent Test**: After making a trivial change, the developer runs the test
command and the lint command; both complete with clear pass/fail output and zero
ambiguous failures on an unmodified scaffold.

**Acceptance Scenarios**:

1. **Given** the scaffolded project, **When** the developer runs the test command,
   **Then** all scaffold-level tests pass and the output clearly indicates the
   number of tests run and their result.
2. **Given** the scaffolded project, **When** the developer runs the lint command,
   **Then** the linter reports zero violations on the unmodified codebase.
3. **Given** a deliberately introduced style violation, **When** the developer runs
   the lint command, **Then** the violation is reported with file, line, and a
   human-readable description.

---

### User Story 3 — Developer Navigates the Project Structure (Priority: P3)

A developer opening the project for the first time can orient themselves within
the codebase by reading a getting-started document that maps the repository
layout to the three architectural layers (content, graph, learning state) defined
in the project constitution.

**Why this priority**: Clear structural documentation reduces onboarding time and
ensures new contributors apply changes in the correct layer, preserving separation
of concerns (Constitution §III).

**Independent Test**: A developer unfamiliar with the project reads the
getting-started document and correctly identifies where to add a new data model,
a new graph query, and a new UI component — without asking for help.

**Acceptance Scenarios**:

1. **Given** the repository root, **When** the developer reads the getting-started
   document, **Then** the document explains the top-level directory layout and maps
   each directory to its architectural layer.
2. **Given** the getting-started document, **When** a developer needs to know where
   to add a new feature in each of the three layers, **Then** the document provides
   a concrete example path for each layer.

---

### Edge Cases

- What happens when the build command is run in a directory that already contains
  partial build output? The build MUST be idempotent and succeed without manual cleanup.
- What happens when the installed build toolchain is below the minimum required
  version? The build MUST fail immediately with a clear message stating the minimum
  version required.
- What happens when cross-compilation targets are built in sequence on the same
  machine? Outputs for different targets MUST not overwrite each other.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The project MUST build to a single distributable binary from the
  repository root using one command.
- **FR-002**: The produced binary MUST run without any external runtime dependencies,
  database servers, or network connectivity.
- **FR-003**: The project MUST support cross-compilation to at least Windows (amd64)
  from a Linux development machine, and MUST produce a native Linux (amd64) binary
  on the same host.
- **FR-004**: The project MUST include a test command that executes all scaffold-level
  tests and reports pass/fail counts.
- **FR-005**: The project MUST include a lint command that enforces a consistent code
  style and reports violations with location and description.
- **FR-006**: The project MUST include a getting-started document covering: build
  instructions, run instructions, test/lint instructions, and directory layout mapped
  to the three architectural layers.
- **FR-007**: The directory structure MUST reflect the three-layer separation of
  concerns: content handling, knowledge graph, and learning state — as defined in
  Constitution §III.
- **FR-008**: The build system MUST validate the minimum required toolchain version
  and fail with a human-readable error if the version requirement is not met.
- **FR-009**: The project MUST include a GitHub Actions workflow that automatically
  runs build, test, and lint on every push to the repository.

### Key Entities

- **Repository root**: Top-level directory containing build scripts, CI configuration,
  and the getting-started document.
- **Content layer**: The sub-tree responsible for reading, writing, and parsing
  `.n4l` plain-text knowledge files.
- **Graph layer**: The sub-tree responsible for knowledge graph storage, querying,
  and graph-building operations.
- **Learning layer**: The sub-tree responsible for scheduling state, review history,
  and spaced-repetition algorithm integration.
- **UI layer**: The sub-tree responsible for the visual interface presented to the
  user.
- **Binary artifact**: The single output file produced by the build command, runnable
  without additional installation steps.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer with only the build toolchain installed completes the full
  build in under 5 minutes on a standard development machine.
- **SC-002**: The produced binary launches and displays the Texere placeholder window in under
  5 seconds on the same machine.
- **SC-003**: The test command completes in under 60 seconds on an unmodified scaffold.
- **SC-004**: 100% of lint rules pass on the unmodified scaffolded codebase (zero
  violations reported).
- **SC-005**: A developer unfamiliar with the project can identify the correct
  directory for a new feature in each architectural layer within 5 minutes of reading
  the getting-started document.
- **SC-006**: The GitHub Actions workflow completes build, test, and lint checks on a push within 10 minutes.

## Clarifications

### Session 2026-04-06

- Q: Which CI platform should the workflow configuration target? → A: GitHub Actions
- Q: What should the application display on first launch (default UI state)? → A: Static placeholder window with "Texere" application name visible
- Q: What is the primary development and CI host OS for cross-compilation? → A: Linux (amd64); cross-compiles to Windows (amd64)

## Assumptions

- The intended build toolchain is available for installation by contributors; its
  own installation procedure is out of scope for this feature.
- The default launch state is a static placeholder window displaying the "Texere"
  application name — full UI implementation is out of scope for this scaffold.
- CI targets the repository's primary remote (GitHub); the repository is assumed to
  already exist on that platform. GitHub Actions is the selected CI platform.
- The primary development and CI host is Linux (amd64). Cross-compilation targets
  Windows (amd64) from Linux. macOS builds are desirable but not a mandatory success
  criterion for this scaffold, as they require a macOS host.
- No user authentication, external API keys, or credentials of any kind are required
  by the scaffold itself.
- Sample or fixture `.n4l` files may be included in the repository for testing
  purposes; they are not considered user data and are intentionally tracked in git.
