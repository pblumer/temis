package xml

import (
	"strings"
	"testing"
)

// TestDecodeDeepNestingRejected asserts pathologically deep XML is refused with
// an error instead of overflowing the stack during the recursive unmarshal
// (audit finding K1, ADR-0008).
func TestDecodeDeepNestingRejected(t *testing.T) {
	var b strings.Builder
	const n = DefaultMaxElementDepth + 100
	for i := 0; i < n; i++ {
		b.WriteString("<a>")
	}
	for i := 0; i < n; i++ {
		b.WriteString("</a>")
	}
	if _, err := Decode([]byte(b.String())); err == nil {
		t.Fatal("expected an error for deeply nested XML, got nil")
	}
}

// TestDecodeModestNestingAccepted guards the ceiling against being set so low it
// would reject realistic documents.
func TestDecodeModestNestingAccepted(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<definitions xmlns="https://www.omg.org/spec/DMN/20240513/MODEL/">`)
	const n = 50
	for i := 0; i < n; i++ {
		b.WriteString("<extensionElements>")
	}
	for i := 0; i < n; i++ {
		b.WriteString("</extensionElements>")
	}
	b.WriteString(`</definitions>`)
	if _, err := Decode([]byte(b.String())); err != nil {
		t.Fatalf("realistic nesting rejected: %v", err)
	}
}
