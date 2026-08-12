export PATH := $(PATH):$(shell go env GOPATH)/bin

.PHONY: build build-windows test test-js bench ci lint dev clean deps

## deps: install Linux system dependencies required for Wails builds
deps:
	sudo apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev gcc-mingw-w64-x86-64-posix pkg-config build-essential

## build: compile Linux amd64 binary → build/bin/texere
build:
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
ci: test test-js bench

## lint: run golangci-lint over all Go source files
lint:
	golangci-lint run ./...

## dev: start Wails development server with hot reload
dev:
	wails dev

## clean: remove compiled binaries and frontend dist
clean:
	rm -rf build/bin frontend/dist
