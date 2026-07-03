package feel

import (
	"strings"
	"testing"
)

// TestParseDeepInputDoesNotCrash asserts the parser rejects pathologically deep
// input with a *ParseError instead of overflowing the goroutine stack and taking
// down the whole process (audit finding K1, ADR-0008). Each case nests far below
// any real DMN model yet above the parse-depth limit.
func TestParseDeepInputDoesNotCrash(t *testing.T) {
	cases := map[string]string{
		"unary minus chain": strings.Repeat("-", 1<<20) + "1",
		"nested parens":     strings.Repeat("(", 1<<20) + "1" + strings.Repeat(")", 1<<20),
		"nested lists":      strings.Repeat("[", 1<<20) + "1" + strings.Repeat("]", 1<<20),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(src)
			if err == nil {
				t.Fatalf("expected a parse error for deep input, got nil")
			}
			if _, ok := err.(*ParseError); !ok {
				t.Fatalf("expected *ParseError, got %T: %v", err, err)
			}
		})
	}
}

// TestParseNormalDepthUnaffected guards against a limit set so low it would
// reject realistic expressions: modest nesting must still parse.
func TestParseNormalDepthUnaffected(t *testing.T) {
	src := strings.Repeat("(", 100) + "1 + 2" + strings.Repeat(")", 100)
	if _, err := Parse(src); err != nil {
		t.Fatalf("realistic nesting rejected: %v", err)
	}
}
