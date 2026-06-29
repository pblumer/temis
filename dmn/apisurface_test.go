package dmn_test

import (
	"bytes"
	"flag"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateAPI = flag.Bool("update-api", false, "rewrite the public API surface golden")

// TestPublicAPISurface pins the exported surface of package dmn — the
// SemVer-stable v1 contract (ADR-0011, ADR-0018, docs/40-api-contract.md §1.5).
// It parses the package, trims everything unexported (including unexported struct
// fields) via ast.FileExports, and compares the rendered declarations to a
// golden file. Any change to the exported surface — a new or removed symbol, a
// changed signature or an added/removed exported field — changes the golden and
// fails here, forcing a conscious decision: additive changes update the golden,
// while a breaking change additionally requires a major version bump.
//
// Run `go test ./dmn -run TestPublicAPISurface -update-api` to record an
// intended change.
func TestPublicAPISurface(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}
	pkg, ok := pkgs["dmn"]
	if !ok {
		t.Fatal("package dmn not found")
	}

	var decls []string
	for _, file := range pkg.Files {
		if !ast.FileExports(file) { // trims to the exported API in place
			continue
		}
		for _, d := range file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				fn.Body = nil // signature only
			}
			var b bytes.Buffer
			if err := printer.Fprint(&b, fset, d); err != nil {
				t.Fatalf("render decl: %v", err)
			}
			decls = append(decls, strings.TrimSpace(b.String()))
		}
	}
	sort.Strings(decls)
	got := strings.Join(decls, "\n\n") + "\n"

	golden := filepath.Join("testdata", "api", "dmn.api")
	if *updateAPI {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", golden)
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read API golden (run with -update-api to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("public API surface of package dmn changed.\n"+
			"If intended: review for breaking changes (ADR-0018 — breaking ⇒ major version),\n"+
			"then re-run with -update-api.\n\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
