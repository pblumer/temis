// Command temis-tck-results runs the official DMN-TCK corpus against the Temis
// engine and writes a vendor result submission in the dmn-tck/tck format:
// a tck_results.csv (one row per test case) and a tck_results.properties
// descriptor. The output is ready to drop into TestResults/Temis/<version>/ of
// a dmn-tck/tck fork and open as a pull request.
//
// Usage:
//
//	TCK_CORPUS=/path/to/tck/TestCases go run ./cmd/temis-tck-results -out ./out
//
// Each row is: "<level>/<suite-dir>","<test-file>","<caseId>","<STATUS>","<note>"
// where STATUS is SUCCESS, FAILURE or NOT_TESTABLE. The external-Java suite
// (0076) is reported NOT_TESTABLE: a pure-Go engine has no JVM to invoke, so the
// cases are not applicable rather than failing (see docs/tck-exceptions.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pblumer/temis/internal/tck"
	"github.com/pblumer/temis/internal/version"
)

// notApplicableSuite marks a suite directory whose cases are not applicable to a
// pure-Go engine and are reported NOT_TESTABLE with a documented reason.
var notApplicable = map[string]string{
	"0076-feel-external-java": "external Java functions require a JVM; not applicable to a pure-Go engine (see docs/tck-exceptions.md)",
}

var levels = []string{"compliance-level-2", "compliance-level-3"}

func main() {
	out := flag.String("out", ".", "directory to write tck_results.csv and tck_results.properties into")
	ver := flag.String("version", version.Version, "product.version recorded in the descriptor and used as the submission folder name")
	date := flag.String("date", "", "last.update date (YYYY-MM-DD); required, since the build has no wall clock")
	flag.Parse()

	root := os.Getenv("TCK_CORPUS")
	if root == "" {
		fatalf("set TCK_CORPUS to a dmn-tck TestCases checkout (see `make tck-corpus`)")
	}
	if *date == "" {
		fatalf("set -date YYYY-MM-DD (the descriptor's last.update field)")
	}
	if err := run(root, *out, *ver, *date); err != nil {
		fatalf("%v", err)
	}
}

func run(root, out, ver, date string) error {
	files := collectSuites(root)
	if len(files) == 0 {
		return fmt.Errorf("no TCK test suites found under %s", root)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	var rows []string
	var nSuccess, nFailure, nNotTestable int
	for _, f := range files {
		dir := filepath.Dir(f)
		rel, _ := filepath.Rel(root, dir) // e.g. compliance-level-2/0001-input-data-string
		suite := filepath.Base(dir)       // e.g. 0001-input-data-string
		model := strings.TrimSuffix(filepath.Base(f), ".xml")
		naNote, na := notApplicable[suite]

		rep, err := tck.RunFile(context.Background(), nil, f)
		if err != nil {
			// A suite that will not load at all: still emit a row per its listed
			// cases if we can, otherwise record the whole suite as one ERROR line.
			rows = append(rows, csvRow(rel, model, "", "ERROR", err.Error()))
			continue
		}
		for _, c := range rep.Results {
			switch {
			case na:
				rows = append(rows, csvRow(rel, model, c.Case, "NOT_TESTABLE", naNote))
				nNotTestable++
			case c.Pass:
				rows = append(rows, csvRow(rel, model, c.Case, "SUCCESS", ""))
				nSuccess++
			default:
				rows = append(rows, csvRow(rel, model, c.Case, "FAILURE", ""))
				nFailure++
			}
		}
	}

	csvPath := filepath.Join(out, "tck_results.csv")
	if err := os.WriteFile(csvPath, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	propPath := filepath.Join(out, "tck_results.properties")
	if err := os.WriteFile(propPath, []byte(descriptor(ver, date)), 0o644); err != nil {
		return err
	}

	total := nSuccess + nFailure + nNotTestable
	fmt.Printf("wrote %s (%d rows)\n", csvPath, len(rows))
	fmt.Printf("wrote %s\n", propPath)
	fmt.Printf("SUCCESS=%d  FAILURE=%d  NOT_TESTABLE=%d  total=%d\n", nSuccess, nFailure, nNotTestable, total)
	if total > 0 {
		fmt.Printf("pass rate over testable cases: %.2f%% (%d/%d)\n",
			100*float64(nSuccess)/float64(nSuccess+nFailure), nSuccess, nSuccess+nFailure)
	}
	return nil
}

func descriptor(ver, date string) string {
	kv := [][2]string{
		{"vendor.name", "Patrick Blumer"},
		{"vendor.url", "https://github.com/pblumer"},
		{"product.name", "Temis"},
		{"product.url", "https://github.com/pblumer/temis"},
		{"product.version", ver},
		{"product.comment", `"Fast, embeddable DMN 1.5 decision engine in Go with full FEEL support"`},
		{"last.update", date},
		{"instructions.url", "https://github.com/pblumer/temis/blob/main/docs/tck-exceptions.md"},
	}
	var b strings.Builder
	for _, e := range kv {
		fmt.Fprintf(&b, "%s=%s\n", e[0], e[1])
	}
	return b.String()
}

// csvRow renders one result row with every field quoted, doubling any embedded
// quote, matching the dmn-tck/tck submission format.
func csvRow(fields ...string) string {
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
	}
	return strings.Join(quoted, ",")
}

func collectSuites(root string) []string {
	var files []string
	for _, level := range levels {
		_ = filepath.WalkDir(filepath.Join(root, level), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if n := d.Name(); strings.Contains(n, "-test-") && strings.HasSuffix(n, ".xml") {
				files = append(files, p)
			}
			return nil
		})
	}
	sort.Strings(files)
	return files
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "temis-tck-results: "+format+"\n", args...)
	os.Exit(1)
}
