package value

import (
	"testing"
	"time"
)

// TestNegativeYearRoundTrip covers parsing (and re-rendering) of negative,
// pre-epoch (BCE / astronomical) years, which the reference time layout cannot
// parse directly. See parseSignedTime.
func TestNegativeYearRoundTrip(t *testing.T) {
	cases := []struct {
		parse func(string) (Value, error)
		in    string
	}{
		{func(s string) (Value, error) { return ParseDate(s) }, "-2021-01-01"},
		{func(s string) (Value, error) { return ParseDateTime(s) }, "-2021-01-01T10:10:10"},
		{func(s string) (Value, error) { return ParseDateTime(s) }, "-2021-01-01T10:10:10+11:00"},
		{func(s string) (Value, error) { return ParseDateTime(s) }, "-2021-01-01T10:10:10@Australia/Melbourne"},
	}
	for _, c := range cases {
		v, err := c.parse(c.in)
		if err != nil {
			t.Fatalf("parse %q: %v", c.in, err)
		}
		if got := v.String(); got != c.in {
			t.Errorf("round-trip %q = %q", c.in, got)
		}
	}
}

// TestEndOfDayMidnight covers the ISO-8601 "24:00:00" end-of-day form, which
// denotes 00:00:00 of the following day.
func TestEndOfDayMidnight(t *testing.T) {
	dt, err := ParseDateTime("2021-01-01T24:00:00")
	if err != nil {
		t.Fatalf("parse 24:00:00: %v", err)
	}
	if got := dt.String(); got != "2021-01-02T00:00:00" {
		t.Errorf("2021-01-01T24:00:00 = %q, want 2021-01-02T00:00:00", got)
	}
}

// TestDateDurationKeepsDateKind checks that date ± duration stays a date with the
// time component dropped (DMN §10.3.2.3.5).
func TestDateDurationKeepsDateKind(t *testing.T) {
	d := NewDate(2021, time.January, 1)
	dt, _ := ParseDuration("PT36H") // 1.5 days
	got := Add(d, dt)
	if got.Kind() != KindDate || got.String() != "2021-01-02" {
		t.Errorf("date + PT36H = %q (%s), want 2021-01-02 (date)", got.String(), got.Kind())
	}
	ym, _ := ParseDuration("P1M")
	if got := Sub(d, ym); got.Kind() != KindDate || got.String() != "2020-12-01" {
		t.Errorf("date - P1M = %q (%s), want 2020-12-01 (date)", got.String(), got.Kind())
	}
}

// TestTemporalSubtraction covers date/date-and-time differences, including the
// null result when the operands disagree on carrying a timezone (a plain date
// counts as zoned/UTC).
func TestTemporalSubtraction(t *testing.T) {
	mustDT := func(s string) DateTime {
		dt, err := ParseDateTime(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return dt
	}
	date := func(y int, m time.Month, d int) Date { return NewDate(y, m, d) }

	cases := []struct {
		name string
		a, b Value
		want string // "" ⇒ expect null
	}{
		{"zoned minus zoned", mustDT("2021-01-02T10:10:10+01:00"), mustDT("2021-01-01T10:10:10+01:00"), "P1DT0H0M0S"},
		{"zoned minus local ⇒ null", mustDT("2021-01-02T10:10:10+01:00"), mustDT("2021-01-01T10:10:10"), ""},
		{"date minus zoned dateAndTime", date(2021, time.January, 2), mustDT("2021-01-01T10:10:10+11:00"), "P1DT0H49M50S"},
		{"date minus local dateAndTime ⇒ null", date(2021, time.January, 2), mustDT("2021-01-01T10:10:10"), ""},
		{"zoned dateAndTime minus date", mustDT("2021-01-02T00:00:00Z"), date(2021, time.January, 2), "P0DT0H0M0S"},
	}
	for _, c := range cases {
		got := Sub(c.a, c.b)
		if c.want == "" {
			if !IsNull(got) {
				t.Errorf("%s: got %s, want null", c.name, got)
			}
			continue
		}
		if IsNull(got) || got.String() != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// TestStringConcatenation covers FEEL's `+` on two strings.
func TestStringConcatenation(t *testing.T) {
	if got := Add(Str("foo"), Str("bar")); got != Str("foobar") {
		t.Errorf(`"foo" + "bar" = %q, want "foobar"`, got)
	}
}

// TestWideYearRoundTrip covers years wider than four digits (up to the FEEL max
// of nine), which the reference layout cannot parse without help.
func TestWideYearRoundTrip(t *testing.T) {
	for _, s := range []string{
		"99999-12-31T11:22:33",
		"999999999-12-31T23:59:59",
		"-999999999-12-31T23:59:59+02:00",
	} {
		dt, err := ParseDateTime(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		if got := dt.String(); got != s {
			t.Errorf("round-trip %q = %q", s, got)
		}
	}
}

// TestFractionalSecondsRendering checks that sub-second precision survives parse
// and render, while whole seconds still elide the fraction.
func TestFractionalSecondsRendering(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2011-12-31T10:15:30.987@Europe/Paris", "2011-12-31T10:15:30.987@Europe/Paris"},
		{"2011-12-31T10:15:30.123456789+02:00", "2011-12-31T10:15:30.123456789+02:00"},
		{"2011-12-31T10:15:30", "2011-12-31T10:15:30"},
	}
	for _, c := range cases {
		dt, err := ParseDateTime(c.in)
		if err != nil {
			t.Fatalf("parse %q: %v", c.in, err)
		}
		if got := dt.String(); got != c.want {
			t.Errorf("render %q = %q, want %q", c.in, got, c.want)
		}
	}
	tm, err := ParseTime("10:15:30.5")
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	if got := tm.String(); got != "10:15:30.5" {
		t.Errorf("time render = %q, want 10:15:30.5", got)
	}
}
