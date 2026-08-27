export PATH := $(PATH):$(shell go env GOPATH)/bin

.PHONY: build build-dev build-windows test test-js bench ci lint lint-frontend dev dev-demo clean deps

## deps: install Linux system dependencies required for Wails builds
deps:
	sudo apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev gcc-mingw-w64-x86-64-posix pkg-config build-essential

## build: compile a RELEASE Linux amd64 binary → build/bin/texere
## No -devtools (that ships the WebKit inspector) and no demo tag.
## Guarded by TestReleaseBuildTargetsCarryNoDevtools / ...NoDemoTag.
build:
	wails build -platform linux/amd64 -tags webkit2_41

## build-dev: same as build, plus the WebKit inspector. Local debugging only —
## never use this to produce a binary for customers.
build-dev:
	wails build -platform linux/amd64 -tags webkit2_41 -devtools

## build-windows: cross-compile Windows amd64 binary → build/bin/texere.exe
build-windows:
	CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 wails build -platform windows/amd64

## test: run all Go tests
test:
	go test ./...

## test-js: run frontend unit tests (node:test)
test-js:
	node --test frontend/src/lib/*.test.js

## bench: run benchmarks and gate on performance targets
bench:
	go test -bench=. -benchtime=3s ./internal/content/... | tee bench.txt
	scripts/check-bench.sh bench.txt

## ci: run tests and benchmarks (used in CI pipelines)
ci: test test-js lint-frontend bench

## lint: run golangci-lint over all Go source files
lint:
	golangci-lint run ./...

## lint-frontend: fail if the Svelte build emits any a11y warning. The compiler
## is the only thing that checks these, so an unread warning is an unenforced
## rule. Suppress a genuine false positive at the call site with an explained
## <!-- svelte-ignore ... --> rather than by loosening this gate.
lint-frontend:
	@cd frontend && npx vite build 2>&1 | tee /tmp/texere-fe-build.log; \
	if grep -q "A11y:" /tmp/texere-fe-build.log; then \
		echo "FAIL: a11y warnings in the Svelte build:"; \
		grep "A11y:" /tmp/texere-fe-build.log; exit 1; \
	else echo "OK: no a11y warnings"; fi

## dev: start Wails development server with hot reload
dev:
	wails dev

## dev-demo: dev server with the demo import seam compiled in, for scripted
## demo recordings. Regenerates frontend/wailsjs with Demo* shims, so it reverts
## them on exit; TestGeneratedBindingsCarryNoDemoShims fails if any slip through.
dev-demo:
	wails dev -tags webkit2_41,demo; git checkout -- frontend/wailsjs/

## clean: remove compiled binaries and frontend dist
clean:
	rm -rf build/bin frontend/dist
