package tck

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// conformanceFloor is the minimum share of applicable official-TCK cases that
// must pass (WP-41). It is a ratchet: it only ever moves up. When a change raises
// the real rate, bump this so a regression trips the gate. It is deliberately a
// floor, not 100 %, because full TCK conformance is reached incrementally and the
// not-yet-supported categories are tracked in docs/tck-exceptions.md.
const conformanceFloor = 90.6

// compliance levels temis targets. Level 2 and 3 are the executable DMN suites;
// the "non-compliant" tree exercises error handling and is out of scope here.
var conformanceLevels = []string{"compliance-level-2", "compliance-level-3"}

// TestOfficialTCKConformance runs the official DMN TCK (github.com/dmn-tck/tck)
// against the public engine and asserts the pass rate stays at or above the
// ratchet floor. It is hermetic-by-skip: set TCK_CORPUS to a checkout of the
// TCK's TestCases directory to run it (the CI "tck" lane clones a pinned commit
// and sets it). Without TCK_CORPUS the test skips, so `go test ./...` stays green
// offline. See docs/50-testing-strategy.md §5 and docs/tck-exceptions.md.
func TestOfficialTCKConformance(t *testing.T) {
	root := os.Getenv("TCK_CORPUS")
	if root == "" {
		t.Skip("set TCK_CORPUS to a dmn-tck TestCases checkout (CI tck lane, or `make tck-corpus`)")
	}

	files := collectSuites(t, root)
	if len(files) == 0 {
		t.Fatalf("no TCK test suites found under %s", root)
	}

	var cases, pass int
	failuresBySuite := map[string]int{}
	for _, f := range files {
		rep, err := RunFile(context.Background(), nil, f)
		if err != nil {
			// A model that will not load at all counts as a full-suite failure so
			// the rate reflects it honestly rather than silently dropping the suite.
			rel, _ := filepath.Rel(root, f)
			failuresBySuite[rel] = -1
			continue
		}
		cases += len(rep.Results)
		pass += rep.Passed()
		if rep.Failed() > 0 {
			rel, _ := filepath.Rel(root, f)
			failuresBySuite[rel] = rep.Failed()
		}
	}
	if cases == 0 {
		t.Fatal("no TCK cases executed")
	}

	rate := 100 * float64(pass) / float64(cases)
	t.Logf("official DMN TCK: %d/%d cases pass (%.2f%%) across %d suites; floor %.1f%%",
		pass, cases, rate, len(files), conformanceFloor)

	// Surface the worst offenders to make regressions and follow-up work legible.
	type sf struct {
		suite string
		fails int
	}
	var worst []sf
	for s, f := range failuresBySuite {
		worst = append(worst, sf{s, f})
	}
	sort.Slice(worst, func(i, j int) bool { return worst[i].fails > worst[j].fails })
	for i, w := range worst {
		if i >= 15 {
			t.Logf("  … and %d more suites with failures", len(worst)-15)
			break
		}
		if w.fails < 0 {
			t.Logf("  LOAD-FAIL %s", w.suite)
		} else {
			t.Logf("  %3d fail  %s", w.fails, w.suite)
		}
	}

	if rate < conformanceFloor {
		t.Fatalf("TCK conformance %.2f%% fell below the ratchet floor %.1f%% — a regression, or lower the floor deliberately", rate, conformanceFloor)
	}
}

func collectSuites(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, level := range conformanceLevels {
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
