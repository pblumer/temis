package value

import (
	"testing"
	"time"
)

func TestParseDateTimeStringRoundTrip(t *testing.T) {
	cases := []struct {
		parse func(string) (Value, error)
		in    string
	}{
		{func(s string) (Value, error) { return ParseDate(s) }, "2024-01-31"},
		{func(s string) (Value, error) { return ParseTime(s) }, "12:30:00"},
		{func(s string) (Value, error) { return ParseTime(s) }, "12:30:00Z"},
		{func(s string) (Value, error) { return ParseTime(s) }, "12:30:00+02:00"},
		{func(s string) (Value, error) { return ParseDateTime(s) }, "2024-01-31T12:30:00"},
		{func(s string) (Value, error) { return ParseDateTime(s) }, "2024-01-31T12:30:00Z"},
		{func(s string) (Value, error) { return ParseDateTime(s) }, "2024-01-31T12:30:00-05:00"},
	}
	for _, c := range cases {
		v, err := c.parse(c.in)
		if err != nil {
			t.Errorf("parse(%q) error: %v", c.in, err)
			continue
		}
		if v.String() != c.in {
			t.Errorf("round-trip %q = %q", c.in, v.String())
		}
	}
}

func TestParseDurationRoundTrip(t *testing.T) {
	cases := map[string]string{
		"P1Y2M":     "P1Y2M",
		"P2Y":       "P2Y0M",
		"P3M":       "P0Y3M",
		"P1DT2H30M": "P1DT2H30M0S",
		"PT90M":     "P0DT1H30M0S",
		"-P1D":      "-P1DT0H0M0S",
		"PT0S":      "P0DT0H0M0S",
	}
	for in, want := range cases {
		v, err := ParseDuration(in)
		if err != nil {
			t.Errorf("ParseDuration(%q) error: %v", in, err)
			continue
		}
		if v.String() != want {
			t.Errorf("ParseDuration(%q).String() = %q, want %q", in, v.String(), want)
		}
	}
}

func TestParseDurationInvalid(t *testing.T) {
	for _, in := range []string{"", "1Y", "P", "PT", "P1Y2D", "PnY", "P1H"} {
		if _, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) = nil error, want error", in)
		}
	}
}

func TestDurationKinds(t *testing.T) {
	ym, _ := ParseDuration("P1Y6M")
	if ym.Kind() != KindYearsMonthsDuration || ym.(YearsMonthsDuration).Months() != 18 {
		t.Errorf("P1Y6M = %v (%s), want 18 months", ym, ym.Kind())
	}
	dt, _ := ParseDuration("P1DT1H")
	if dt.Kind() != KindDaysTimeDuration || dt.(DaysTimeDuration).Duration() != 25*time.Hour {
		t.Errorf("P1DT1H = %v (%s), want 25h", dt, dt.Kind())
	}
}

func TestDateMonthArithmeticClamps(t *testing.T) {
	// Adding one month to Jan 31 clamps to the last day of February (leap year).
	d := NewDate(2024, time.January, 31)
	ym, _ := ParseDuration("P1M")
	got := Add(d, ym)
	if got.String() != "2024-02-29" {
		t.Errorf("2024-01-31 + P1M = %s, want 2024-02-29", got)
	}
}

func TestDateDifferenceIsDuration(t *testing.T) {
	a := NewDate(2024, time.January, 10)
	b := NewDate(2024, time.January, 1)
	got := Sub(a, b)
	d, ok := got.(DaysTimeDuration)
	if !ok || d.Duration() != 9*24*time.Hour {
		t.Errorf("date difference = %v, want P9D", got)
	}
}

func TestDateTimePlusDaysTimeDuration(t *testing.T) {
	dt, _ := ParseDateTime("2024-01-31T23:00:00")
	dur, _ := ParseDuration("PT2H")
	got := Add(dt, dur)
	if got.String() != "2024-02-01T01:00:00" {
		t.Errorf("datetime + PT2H = %s, want 2024-02-01T01:00:00", got)
	}
}

func TestDurationScaling(t *testing.T) {
	dt, _ := ParseDuration("PT1H")
	if got := Mul(dt, MustNumber("2")).(DaysTimeDuration).Duration(); got != 2*time.Hour {
		t.Errorf("PT1H * 2 = %v, want 2h", got)
	}
	ym, _ := ParseDuration("P1Y")
	if got := Mul(ym, MustNumber("2")).(YearsMonthsDuration).Months(); got != 24 {
		t.Errorf("P1Y * 2 = %d months, want 24", got)
	}
	// duration / duration yields a number
	a, _ := ParseDuration("PT3H")
	b, _ := ParseDuration("PT1H")
	if got := Div(a, b); IsNull(got) || got.String() != "3" {
		t.Errorf("PT3H / PT1H = %s, want 3", got)
	}
}

func TestTemporalNullPropagation(t *testing.T) {
	d := NewDate(2024, time.January, 1)
	if !IsNull(Add(d, MustNumber("1"))) { // date + number is undefined
		t.Error("date + number should be null")
	}
	if !IsNull(Mul(d, MustNumber("2"))) {
		t.Error("date * number should be null")
	}
}
