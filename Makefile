# Temis — build & quality targets.
# `make verify` is the authoritative local & CI gate (see docs/50-testing-strategy.md).

GO      ?= go
PKGS    ?= ./...

.PHONY: all verify fmt fmt-check vet lint test bench tck build tidy clean help

all: verify

## verify: full gate — formatting, vet, lint, race tests, bench & tck smoke
verify: fmt-check vet lint test bench tck

## fmt: format all Go sources in place
fmt:
	$(GO) fmt $(PKGS)

## fmt-check: fail if any Go source is not gofmt-clean
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

## vet: run go vet
vet:
	$(GO) vet $(PKGS)

## lint: run golangci-lint (no-op-friendly if not installed)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found; skipping (install: https://golangci-lint.run)"; \
	fi

## test: run the full test suite with the race detector
test:
	$(GO) test $(PKGS) -race

## bench: smoke-run all benchmarks without running tests
bench:
	$(GO) test -run=^$$ -bench=. -benchmem $(PKGS)

## tck: run the TCK runner package (tolerant while no cases exist yet)
tck:
	$(GO) test ./internal/tck/...

## build: compile all packages and binaries
build:
	$(GO) build $(PKGS)

## feel-spike: build the FEEL-WASM spike (ADR-0016 Gate 2) into web/feel-spike/
feel-spike:
	./web/feel-spike/build.sh

## web: build the embedded modeler frontend (ADR-0016 WP-60) into web/dist/
web:
	cd web && npm ci && npm run build

## web-check: type-check the frontend without emitting (CI frontend lane)
web-check:
	cd web && npm ci && npm run typecheck

## tidy: tidy go.mod/go.sum
tidy:
	$(GO) mod tidy

## clean: remove build/test artifacts
clean:
	$(GO) clean
	rm -f coverage.out

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
