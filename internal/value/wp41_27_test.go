package value

import (
	"testing"
	"time"
)

// TestParseDurationFractionalSeconds covers fractional and empty-fraction
// seconds in a lexical days-and-time duration (TCK 1120). A trailing "." with no
// digits (PT0.S) is accepted as a zero fraction.
func TestParseDurationFractionalSeconds(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"PT0.000S", 0},
		{"PT0.S", 0},
		{"PT0.5S", 500 * time.Millisecond},
		{"PT1.25S", 1250 * time.Millisecond},
		{"P1DT2H3M4.5S", 24*time.Hour + 2*time.Hour + 3*time.Minute + 4500*time.Millisecond},
	}
	for _, c := range cases {
		d, err := ParseDuration(c.in)
		if err != nil {
			t.Errorf("ParseDuration(%q) error: %v", c.in, err)
			continue
		}
		dtd, ok := d.(DaysTimeDuration)
		if !ok {
			t.Errorf("ParseDuration(%q) = %T, want DaysTimeDuration", c.in, d)
			continue
		}
		if dtd.d != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, dtd.d, c.want)
		}
	}
}

// TestParseDurationFractionOnlyOnSeconds rejects a decimal point on a non-second
// unit.
func TestParseDurationFractionOnlyOnSeconds(t *testing.T) {
	for _, in := range []string{"PT1.5M", "P1.5D", "PT1.5H"} {
		if _, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) should error (fraction only allowed on seconds)", in)
		}
	}
}

// TestScaleDurationTruncatesTowardZero covers duration × number: the result is
// truncated toward zero to the duration's integral unit (TCK 0100), so
// -2.5 * P1Y11M (23 months) = -57.5 → -P4Y9M (-57), not -58.
func TestScaleDurationTruncatesTowardZero(t *testing.T) {
	ym, _ := ParseDuration("P1Y11M") // 23 months
	nHalf, err := ParseNumber("-2.5")
	if err != nil {
		t.Fatal(err)
	}
	got := Mul(nHalf, ym)
	ymd, ok := got.(YearsMonthsDuration)
	if !ok {
		t.Fatalf("Mul returned %T, want YearsMonthsDuration", got)
	}
	if ymd.months != -57 {
		t.Errorf("-2.5 * P1Y11M = %d months, want -57", ymd.months)
	}
}
