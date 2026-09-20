package main

import (
	"strings"
	"testing"
)

func TestCSVRow(t *testing.T) {
	got := csvRow("compliance-level-2/0001-input-data-string", "0001-input-data-string-test-01", "001", "SUCCESS", "")
	want := `"compliance-level-2/0001-input-data-string","0001-input-data-string-test-01","001","SUCCESS",""`
	if got != want {
		t.Errorf("csvRow =\n  %s\nwant\n  %s", got, want)
	}
	// Embedded quotes are doubled, per CSV.
	if got := csvRow(`a"b`); got != `"a""b"` {
		t.Errorf(`csvRow(a"b) = %s, want "a""b"`, got)
	}
}

func TestDescriptor(t *testing.T) {
	d := descriptor("1.2.3", "2026-07-05")
	for _, want := range []string{
		"product.name=Temis\n",
		"product.version=1.2.3\n",
		"last.update=2026-07-05\n",
		"vendor.name=Patrick Blumer\n",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("descriptor missing %q; got:\n%s", want, d)
		}
	}
}
