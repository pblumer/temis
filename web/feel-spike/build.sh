#!/usr/bin/env sh
# Build the FEEL-WASM spike (ADR-0016, Gate 2).
# Produces feel.wasm next to index.html and copies Go's wasm_exec.js loader.
# Both artifacts are generated (git-ignored); commit only the sources.
set -eu

dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH= cd -- "$dir/../.." && pwd)

echo "building feel.wasm …"
GOOS=js GOARCH=wasm go build -o "$dir/feel.wasm" "$root/cmd/feel-wasm"

echo "copying wasm_exec.js …"
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$dir/wasm_exec.js"

echo "done. serve it:  go run ./cmd/feel-spike-serve   (then open http://localhost:8090)"
