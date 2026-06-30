# Temis — build & quality targets.
# `make verify` is the authoritative local & CI gate (see docs/50-testing-strategy.md).

GO      ?= go
PKGS    ?= ./...
FUZZTIME ?= 10s

.PHONY: all verify fmt fmt-check vet lint test bench budget tck fuzz proto proto-check build tidy clean help web web-wasm web-check web-e2e

# Pinned codegen tools (ADR-0020). go-1.23-compatible versions.
CONNECT_VERSION ?= v1.18.1
PROTOC_GEN_GO_VERSION ?= v1.36.6

all: verify

## verify: full gate — formatting, vet, lint, race tests, bench, budget & tck
verify: fmt-check vet lint test bench budget tck

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

## budget: performance-budget CI gate, run without the race detector (WP-42, docs/50-testing-strategy.md §6)
budget:
	$(GO) test -run '^TestPerformanceBudget$$' -count=1 ./dmn/

## tck: run the TCK runner package (tolerant while no cases exist yet)
tck:
	$(GO) test ./internal/tck/...

## fuzz: run every fuzz target for FUZZTIME each, asserting no crash (WP-44, docs/50-testing-strategy.md §3)
fuzz:
	@set -e; \
	for spec in \
		"./dmn:FuzzCompile" \
		"./internal/xml:FuzzDecode" \
		"./internal/value:FuzzParseNumber" \
		"./internal/value:FuzzParseDuration" \
		"./internal/feel:FuzzLexer" \
		"./internal/feel:FuzzParser" \
		"./internal/feel:FuzzBoundedEvaluation"; do \
		pkg=$${spec%%:*}; fn=$${spec##*:}; \
		echo "=== fuzz $$fn ($$pkg) for $(FUZZTIME) ==="; \
		$(GO) test -run='^$$' -fuzz="^$$fn$$" -fuzztime=$(FUZZTIME) $$pkg; \
	done

## proto-tools: install the pinned protobuf/connect codegen plugins into GOBIN
proto-tools:
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	$(GO) install connectrpc.com/connect/cmd/protoc-gen-connect-go@$(CONNECT_VERSION)

## proto: regenerate the gRPC code from proto/ (needs buf + the proto-tools on PATH)
proto:
	buf generate

## proto-check: fail if the committed gRPC code is stale (no-op-friendly if buf absent)
proto-check:
	@if command -v buf >/dev/null 2>&1; then \
		buf generate && git diff --exit-code -- internal/gen \
			|| { echo "generated proto code is stale; run 'make proto'"; exit 1; }; \
	else \
		echo "buf not found; skipping proto drift check (install: https://buf.build)"; \
	fi

## build: compile all packages and binaries
build:
	$(GO) build $(PKGS)

## feel-spike: build the FEEL-WASM spike (ADR-0016 Gate 2) into web/feel-spike/
feel-spike:
	./web/feel-spike/build.sh

## web-wasm: build the FEEL validator (cmd/feel-wasm) into web/public/ for the modeler
web-wasm:
	GOOS=js GOARCH=wasm $(GO) build -o web/public/feel.wasm ./cmd/feel-wasm
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" web/public/wasm_exec.js

## web: build the embedded modeler frontend (ADR-0016 WP-60) into web/dist/
web: web-wasm
	cd web && npm ci && npm run build

## web-check: type-check the frontend without emitting (CI frontend lane)
web-check:
	cd web && npm ci && npm run typecheck

## web-e2e: build the frontend and run the Playwright end-to-end tests (browser)
web-e2e: web
	cd web && npx playwright test

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
