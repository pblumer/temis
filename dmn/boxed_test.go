package dmn_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// compileModel loads and compiles a testdata model, failing on any error or
// compile diagnostic.
func compileModel(t *testing.T, file string) *dmn.Definitions {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "models", file))
	if err != nil {
		t.Fatalf("read model: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), data)
	if err != nil {
		t.Fatalf("compile %s: %v", file, err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile diagnostics for %s: %+v", file, diags)
	}
	return defs
}

func evalDecision(t *testing.T, defs *dmn.Definitions, name string, in dmn.Input) any {
	t.Helper()
	dec, err := defs.Decision(name)
	if err != nil {
		t.Fatalf("decision %q: %v", name, err)
	}
	res, err := dec.Evaluate(context.Background(), in)
	if err != nil {
		t.Fatalf("evaluate %q: %v", name, err)
	}
	return res.Outputs[name]
}

// TestBoxedContext covers WP-23: a boxed context with a result cell (later
// entries build on earlier ones) and one without (value is a context).
func TestBoxedContext(t *testing.T) {
	defs := compileModel(t, "boxed_context_15.dmn")

	if got := evalDecision(t, defs, "Score", dmn.Input{"Points": 5}); got != "20" {
		t.Errorf("Score(Points=5) = %v, want 20", got)
	}

	got := evalDecision(t, defs, "Profile", dmn.Input{"Points": 5})
	want := map[string]any{"Doubled": "10", "Plus": "6"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Profile(Points=5) = %#v, want %#v", got, want)
	}
}

// TestBKMInvocation covers WP-23/WP-24: invoking a business knowledge model by
// named binding, and calling the same BKM from a literal expression.
func TestBKMInvocation(t *testing.T) {
	defs := compileModel(t, "bkm_invocation_15.dmn")

	cases := []struct {
		decision string
		total    int
		want     string
	}{
		{"Order Discount", 1500, "0.2"},
		{"Order Discount", 500, "0.1"},
		{"Quick Discount", 1500, "0.2"},
		{"Quick Discount", 500, "0.1"},
	}
	for _, c := range cases {
		got := evalDecision(t, defs, c.decision, dmn.Input{"Order Total": c.total})
		if got != c.want {
			t.Errorf("%s(Order Total=%d) = %v, want %s", c.decision, c.total, got, c.want)
		}
	}
}

// TestRecursiveBKM covers WP-24: a business knowledge model that calls itself.
func TestRecursiveBKM(t *testing.T) {
	defs := compileModel(t, "recursion_15.dmn")

	for _, c := range []struct {
		n    int
		want string
	}{{1, "1"}, {5, "120"}, {6, "720"}} {
		if got := evalDecision(t, defs, "Factorial", dmn.Input{"N": c.n}); got != c.want {
			t.Errorf("Factorial(N=%d) = %v, want %s", c.n, got, c.want)
		}
	}
}
