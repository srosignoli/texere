# Build Contract: Project Scaffold

**Branch**: `001-project-scaffold`
**Date**: 2026-04-06

The scaffold's external contract is the **developer build interface** — the commands
a contributor uses to build, test, lint, and run the application. These commands are
stable and MUST NOT be renamed or removed without a plan amendment.

---

## Make Targets

### `make build`

Produces a native Linux (amd64) binary.

```
Input:   Source tree at repository root
Output:  build/bin/texere  (Linux ELF amd64)
Success: Exit code 0; binary exists at build/bin/texere
Failure: Non-zero exit; error written to stderr with cause
```

**Constraints**:
- MUST complete in under 5 minutes on a standard development machine (SC-001)
- MUST produce a single self-contained binary with no external runtime dependencies
- MUST fail with a human-readable error if Go < 1.21 is detected

---

### `make build-windows`

Cross-compiles a Windows (amd64) binary from a Linux host.

```
Input:   Source tree; mingw-w64 cross-compiler on PATH
Output:  build/bin/texere.exe  (Windows PE amd64)
Success: Exit code 0; binary exists at build/bin/texere.exe
Failure: Non-zero exit; if mingw-w64 missing, error MUST name the missing tool
```

**Constraints**:
- Requires `x86_64-w64-mingw32-gcc` on PATH (available via `apt-get install gcc-mingw-w64-x86-64`)
- Output MUST NOT overwrite `build/bin/texere` (Linux binary)

---

### `make test`

Runs all Go tests in the repository.

```
Input:   Source tree
Output:  Test result summary to stdout (pass count, fail count, elapsed)
Success: Exit code 0; all tests pass
Failure: Non-zero exit; failing test names and locations written to stdout
```

**Constraints**:
- MUST complete in under 60 seconds on an unmodified scaffold (SC-003)
- MUST report the number of tests run

---

### `make lint`

Runs the Go linter over all Go source files.

```
Input:   Source tree; .golangci.yml configuration
Output:  Zero output on pass; violations written to stdout with file:line:description
Success: Exit code 0; zero violations
Failure: Non-zero exit; each violation includes file path, line number, rule name,
         and human-readable description
```

**Constraints**:
- MUST report zero violations on the unmodified scaffold (SC-004)
- `frontend/` directory MUST be excluded from Go linting

---

### `make dev`

Starts the Wails development server with hot reload.

```
Input:   Source tree; wails CLI on PATH
Output:  Application window opens; file changes trigger hot reload
Success: Application window visible; console shows "Wails dev server started"
Failure: Non-zero exit with cause (e.g., port conflict, missing frontend deps)
```

**Constraints**:
- Out of scope for CI; for local development only

---

### `make clean`

Removes all build artifacts.

```
Input:   Repository root
Output:  build/bin/ emptied; frontend/dist/ removed
Success: Exit code 0; no artifact files remain
Failure: Non-zero exit only on permission errors
```

---

## GitHub Actions Workflow Contract

**File**: `.github/workflows/ci.yml`

**Trigger**: Every push to any branch; every pull request to `main`

**Jobs**:

| Job | Runner | Steps |
|---|---|---|
| `build-and-test` | `ubuntu-latest` | Install Go 1.21, install wails, `make build`, `make test`, `make lint` |
| `build-windows` | `ubuntu-latest` | Install Go 1.21, install mingw-w64, install wails, `make build-windows` |

**Success**: Both jobs complete with exit code 0 within 10 minutes (SC-006)

**Failure**: Job name, step name, and error output visible in GitHub Actions UI

---

## Binary Launch Contract

**Binary**: `build/bin/texere` (Linux) or `build/bin/texere.exe` (Windows)

```
Input:   No arguments required; no configuration files required
Output:  OS window opens displaying "Texere" application name
Success: Window visible within 5 seconds of launch (SC-002)
Failure: Non-zero exit with error message to stderr
```
