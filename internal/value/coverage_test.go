package value

import (
	"testing"
	"time"
)

// TestIsValueMarkers exercises the no-op isValue marker methods on every Value
// implementation so the type-set membership assertions are covered. They have no
// behaviour beyond satisfying the Value interface.
func TestIsValueMarkers(t *testing.T) {
	values := []Value{
		nullValue{},
		Bool(true),
		Number{dec: MustNumber("1").dec},
		Str("x"),
		NewDate(2024, time.January, 1),
		NewTime(1, 2, 3, 0, nil),
		NewDateTime(time.Now()),
		NewDaysTimeDuration(time.Hour),
		NewYearsMonthsDuration(1),
		NewList(),
		NewContext(),
		Range{Low: Null, High: Null},
		&Function{},
	}
	for _, v := range values {
		v.isValue()
		// Kind/String must also be callable on every implementation.
		_ = v.Kind()
		_ = v.String()
	}
}

// TestNumberIntegralMethods covers Floor, Ceiling, Abs, Int64 and roundIntegral.
func TestNumberIntegralMethods(t *testing.T) {
	floorCases := []struct{ in, want string }{
		{"3.7", "3"},
		{"-3.2", "-4"},
		{"5", "5"},
	}
	for _, c := range floorCases {
		if got := MustNumber(c.in).Floor().String(); got != c.want {
			t.Errorf("Floor(%s) = %s, want %s", c.in, got, c.want)
		}
	}

	ceilCases := []struct{ in, want string }{
		{"3.2", "4"},
		{"-3.7", "-3"},
		{"5", "5"},
	}
	for _, c := range ceilCases {
		if got := MustNumber(c.in).Ceiling().String(); got != c.want {
			t.Errorf("Ceiling(%s) = %s, want %s", c.in, got, c.want)
		}
	}

	absCases := []struct{ in, want string }{
		{"-7", "7"},
		{"7", "7"},
		{"0", "0"},
	}
	for _, c := range absCases {
		if got := MustNumber(c.in).Abs().String(); got != c.want {
			t.Errorf("Abs(%s) = %s, want %s", c.in, got, c.want)
		}
	}

	if i, ok := MustNumber("42").Int64(); !ok || i != 42 {
		t.Errorf("Int64(42) = %d ok=%v, want 42 true", i, ok)
	}
	if i, ok := MustNumber("-9").Int64(); !ok || i != -9 {
		t.Errorf("Int64(-9) = %d ok=%v, want -9 true", i, ok)
	}
	// A non-integral value does not fit exactly as an int64.
	if _, ok := MustNumber("3.5").Int64(); ok {
		t.Error("Int64(3.5) should report ok=false")
	}
	// A value beyond int64 range does not fit either.
	if _, ok := MustNumber("99999999999999999999999999").Int64(); ok {
		t.Error("Int64(huge) should report ok=false")
	}
}

// TestNumberFloorToCeilingTo covers the scaled floor/ceiling helpers.
func TestNumberFloorToCeilingTo(t *testing.T) {
	if r, ok := MustNumber("1.234").FloorTo(2); !ok || r.String() != "1.23" {
		t.Errorf("FloorTo(1.234,2) = %v ok=%v, want 1.23", r, ok)
	}
	if r, ok := MustNumber("-1.234").FloorTo(2); !ok || r.String() != "-1.24" {
		t.Errorf("FloorTo(-1.234,2) = %v ok=%v, want -1.24", r, ok)
	}
	if r, ok := MustNumber("1.231").CeilingTo(2); !ok || r.String() != "1.24" {
		t.Errorf("CeilingTo(1.231,2) = %v ok=%v, want 1.24", r, ok)
	}
	if r, ok := MustNumber("-1.236").CeilingTo(2); !ok || r.String() != "-1.23" {
		t.Errorf("CeilingTo(-1.236,2) = %v ok=%v, want -1.23", r, ok)
	}
}

// TestTimeOfAndDateAccessors covers TimeOf, Date.Time and DateTime.Time.
func TestTimeOfAndDateAccessors(t *testing.T) {
	dt, _ := ParseDateTime("2024-03-04T13:30:45+02:00")
	tm := TimeOf(dt)
	if tm.Kind() != KindTime || tm.String() != "13:30:45+02:00" {
		t.Errorf("TimeOf = %q (%s), want 13:30:45+02:00", tm.String(), tm.Kind())
	}

	d := NewDate(2024, time.March, 4)
	if d.Time().Year() != 2024 || d.Time().Month() != time.March {
		t.Errorf("Date.Time() = %v, want 2024-03", d.Time())
	}

	if dt.Time().Hour() != 13 {
		t.Errorf("DateTime.Time().Hour() = %d, want 13", dt.Time().Hour())
	}
}

// TestNewDateTimeRendersZoned covers NewDateTime and the zoned suffix path.
func TestNewDateTimeRendersZoned(t *testing.T) {
	utc := NewDateTime(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	if utc.String() != "2024-01-02T03:04:05Z" {
		t.Errorf("NewDateTime(UTC) = %q, want ...Z", utc.String())
	}
	loc := time.FixedZone("", 3*3600)
	off := NewDateTime(time.Date(2024, 1, 2, 3, 4, 5, 0, loc))
	if off.String() != "2024-01-02T03:04:05+03:00" {
		t.Errorf("NewDateTime(+03:00) = %q", off.String())
	}
}

// TestPowEdgeCases covers the pow invalid-operation/overflow branch.
func TestPowEdgeCases(t *testing.T) {
	// The square root of a negative base is an invalid operation -> null.
	if got := Exp(MustNumber("-2"), MustNumber("0.5")); !IsNull(got) {
		t.Errorf("-2 ** 0.5 = %s, want null", got)
	}
	// A power that overflows the exponent range yields null.
	if got := Exp(MustNumber("10"), MustNumber("1000000000")); !IsNull(got) {
		t.Errorf("10 ** 1e9 = %s, want null", got)
	}
}

// TestSqrtExpInvalid covers the unary non-finite / invalid branches.
func TestSqrtExpInvalid(t *testing.T) {
	// exp of a very large number overflows -> null.
	if _, ok := MustNumber("1000000000").Exp(); ok {
		t.Error("exp(1e9) should be invalid (null)")
	}
}

// TestModuloOfNegativeFraction exercises more Modulo paths.
func TestModuloOfNegativeFraction(t *testing.T) {
	if r, ok := MustNumber("-5.5").Modulo(MustNumber("2")); !ok || r.String() != "0.5" {
		t.Errorf("modulo(-5.5,2) = %v ok=%v, want 0.5", r, ok)
	}
}

// TestParseNumberOutOfRange covers the rounding/out-of-range error path.
func TestParseNumberOutOfRange(t *testing.T) {
	// An exponent beyond the context range cannot be rounded -> error.
	if _, err := ParseNumber("1E1000000000"); err == nil {
		t.Error("ParseNumber(1E1000000000) should error")
	}
}

// TestMustNumberPanics covers the panic branch of MustNumber.
func TestMustNumberPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustNumber(garbage) should panic")
		}
	}()
	_ = MustNumber("not-a-number")
}

// TestEqualTemporalAndDurationKinds covers the Equal branches for time,
// date-time and both duration kinds, plus null/non-null mismatch.
func TestEqualTemporalAndDurationKinds(t *testing.T) {
	tA, _ := ParseTime("12:00:00")
	tB, _ := ParseTime("12:00:00")
	if Equal(tA, tB) != True {
		t.Error("equal times should be equal")
	}
	dtA, _ := ParseDateTime("2024-01-01T00:00:00")
	dtB, _ := ParseDateTime("2024-01-01T00:00:00")
	if Equal(dtA, dtB) != True {
		t.Error("equal date-times should be equal")
	}
	ym1 := NewYearsMonthsDuration(12)
	ym2 := NewYearsMonthsDuration(12)
	if Equal(ym1, ym2) != True {
		t.Error("equal years-months durations should be equal")
	}
	dt1 := NewDaysTimeDuration(time.Hour)
	dt2 := NewDaysTimeDuration(time.Hour)
	if Equal(dt1, dt2) != True {
		t.Error("equal days-time durations should be equal")
	}
	// boolean equality false branch
	if Equal(True, False) != False {
		t.Error("true != false")
	}
	// a context compared with non-matching key set
	ca := NewContext().Put("x", MustNumber("1"))
	cb := NewContext().Put("y", MustNumber("1"))
	if Equal(ca, cb) != False {
		t.Error("contexts with disjoint keys should be unequal")
	}
	// lists of differing length
	if Equal(NewList(MustNumber("1")), NewList(MustNumber("1"), MustNumber("2"))) != False {
		t.Error("lists of different length should be unequal")
	}
}

// TestCompareTemporalEqualAndAfter covers cmpTime equal/after branches and the
// cmpInt64 equal branch via durations.
func TestCompareTemporalEqualAndAfter(t *testing.T) {
	a := NewDate(2024, time.February, 1)
	b := NewDate(2024, time.January, 1)
	if c, ok := Compare(a, b); !ok || c != 1 {
		t.Errorf("Feb > Jan = (%d,%v), want (1,true)", c, ok)
	}
	if c, ok := Compare(a, a); !ok || c != 0 {
		t.Errorf("same date = (%d,%v), want (0,true)", c, ok)
	}
	d1 := NewDaysTimeDuration(time.Hour)
	d2 := NewDaysTimeDuration(time.Hour)
	if c, ok := Compare(d1, d2); !ok || c != 0 {
		t.Errorf("equal durations = (%d,%v), want (0,true)", c, ok)
	}
}

// TestScaleDurationOverflowAndDefault covers scaleDuration / scaleInt64 failure
// and the non-duration default branch.
func TestScaleDurationOverflowAndDefault(t *testing.T) {
	// Multiplying a large duration by a large number overflows int64 -> null.
	big := NewDaysTimeDuration(time.Duration(1) << 62)
	if got := Mul(big, MustNumber("1000")); !IsNull(got) {
		t.Errorf("huge duration * 1000 = %s, want null", got)
	}
	// Years-months overflow on scale.
	bigYM := NewYearsMonthsDuration(1 << 60)
	if got := Mul(bigYM, MustNumber("1000")); !IsNull(got) {
		t.Errorf("huge ym * 1000 = %s, want null", got)
	}
}

// TestSuffixOffsetForms covers the zone suffix offset (non-Z) branch and the
// unzoned empty branch.
func TestSuffixOffsetForms(t *testing.T) {
	z := zone{zoned: true}
	tNeg := time.Date(1, 1, 1, 0, 0, 0, 0, time.FixedZone("", -5*3600))
	if got := z.suffix(tNeg); got != "-05:00" {
		t.Errorf("suffix(-05:00) = %q", got)
	}
	zUnzoned := zone{zoned: false}
	if got := zUnzoned.suffix(tNeg); got != "" {
		t.Errorf("unzoned suffix = %q, want empty", got)
	}
}

// TestParseDateTimeErrors covers the malformed-clock and zone-error branches of
// ParseDateTime and findOffset/parseOffset failure paths.
func TestParseDateTimeErrors(t *testing.T) {
	// 'T' present but body unparseable.
	if _, err := ParseDateTime("2024-99-99T00:00:00"); err == nil {
		t.Error("ParseDateTime with bad date should error")
	}
	// invalid @zone after T.
	if _, err := ParseDateTime("2024-01-01T00:00:00@No/Where"); err == nil {
		t.Error("ParseDateTime with unknown zone should error")
	}
	// malformed offset.
	if _, err := ParseTime("12:00:00+99:99"); err == nil {
		t.Error("ParseTime with bad offset should error")
	}
}

// TestScanComponentsErrors covers scanComponents error branches via ParseDuration.
func TestScanComponentsErrors(t *testing.T) {
	for _, in := range []string{"PYM", "P1", "P1X", "P9999999999999999999999D"} {
		if _, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) should error", in)
		}
	}
}

// TestAddMonthsNegativeUnderflow covers the addMonths month<1 wrap branch.
func TestAddMonthsNegativeUnderflow(t *testing.T) {
	d := NewDate(2024, time.January, 15)
	ym, _ := ParseDuration("P2M")
	// Subtracting 2 months from January wraps to the previous year.
	if got := Sub(d, ym).String(); got != "2023-11-15" {
		t.Errorf("2024-01-15 - P2M = %s, want 2023-11-15", got)
	}
}

// TestDatePlusDaysTimeDuration covers shift's Date + DaysTimeDuration branch.
// Per DMN, date ± duration stays a date: the duration is applied and the result
// is truncated back to the calendar day (PT6H stays inside 2024-01-01).
func TestDatePlusDaysTimeDuration(t *testing.T) {
	d := NewDate(2024, time.January, 1)
	dur, _ := ParseDuration("PT6H")
	got := Add(d, dur)
	if got.Kind() != KindDate || got.String() != "2024-01-01" {
		t.Errorf("date + PT6H = %q (%s), want 2024-01-01 (date)", got.String(), got.Kind())
	}
}

// TestMulTwoNonNumbers covers the Mul fall-through that yields null when neither
// operand is a number (e.g. date * date).
func TestMulTwoNonNumbers(t *testing.T) {
	a := NewDate(2024, time.January, 1)
	b := NewDate(2024, time.February, 1)
	if got := Mul(a, b); !IsNull(got) {
		t.Errorf("date * date = %s, want null", got)
	}
}

// TestDivDurationByZeroDuration covers the like-duration division-by-zero
// branches in Div.
func TestDivDurationByZeroDuration(t *testing.T) {
	dt := NewDaysTimeDuration(time.Hour)
	zeroDT := NewDaysTimeDuration(0)
	if got := Div(dt, zeroDT); !IsNull(got) {
		t.Errorf("PT1H / PT0S = %s, want null", got)
	}
	ym := NewYearsMonthsDuration(12)
	zeroYM := NewYearsMonthsDuration(0)
	if got := Div(ym, zeroYM); !IsNull(got) {
		t.Errorf("P1Y / P0M = %s, want null", got)
	}
}

// TestEqualAndCompareDateTimeBranches covers the KindDate Equal branch and the
// KindTime/KindDateTime Compare branches.
func TestEqualAndCompareDateTimeBranches(t *testing.T) {
	d1 := NewDate(2024, time.January, 1)
	d2 := NewDate(2024, time.January, 1)
	if Equal(d1, d2) != True {
		t.Error("equal dates should be equal")
	}
	tA, _ := ParseTime("10:00:00")
	tB, _ := ParseTime("11:00:00")
	if c, ok := Compare(tA, tB); !ok || c != -1 {
		t.Errorf("10:00 < 11:00 = (%d,%v), want (-1,true)", c, ok)
	}
	dtA, _ := ParseDateTime("2024-01-01T00:00:00")
	dtB, _ := ParseDateTime("2024-01-02T00:00:00")
	if c, ok := Compare(dtA, dtB); !ok || c != -1 {
		t.Errorf("datetime compare = (%d,%v), want (-1,true)", c, ok)
	}
}

// TestZoneSuffixNamed covers the named-zone branch of zone.suffix via a parsed
// IANA-zoned time round-trip.
func TestZoneSuffixNamed(t *testing.T) {
	tm, err := ParseTime("12:00:00@Europe/Paris")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := tm.String(); got != "12:00:00@Europe/Paris" {
		t.Errorf("named-zone time = %q", got)
	}
}

// TestFindOffsetShortString covers findOffset's short-string guard and the
// no-offset local-time fall-through.
func TestFindOffsetShortString(t *testing.T) {
	// A 5-character body is shorter than the 6-char offset window.
	if _, err := ParseTime("12:00"); err == nil {
		t.Error("ParseTime(12:00) should error (no seconds)")
	}
}

// TestParseDurationMoreErrors covers the empty-time-part branch and a negative
// years-months parse.
func TestParseDurationMoreErrors(t *testing.T) {
	if _, err := ParseDuration("P1YT"); err == nil {
		t.Error("ParseDuration(P1YT) with empty time part should error")
	}
	v, err := ParseDuration("-P1Y6M")
	if err != nil {
		t.Fatalf("parse -P1Y6M: %v", err)
	}
	if v.(YearsMonthsDuration).Months() != -18 {
		t.Errorf("-P1Y6M = %d months, want -18", v.(YearsMonthsDuration).Months())
	}
}

// TestRangeStringClosedHigh covers the HighClosed branch of Range.String.
func TestRangeStringClosedHigh(t *testing.T) {
	r := Range{LowClosed: false, Low: MustNumber("1"), High: MustNumber("10"), HighClosed: true}
	if got := r.String(); got != "(1..10]" {
		t.Errorf("Range.String() = %q, want (1..10]", got)
	}
}

// TestMemberPartialBranches covers the remaining Member branches: unknown
// property names, days-time "seconds", years-months "years"/"months" edges, and
// zoned-time offset on a Time value.
func TestMemberPartialBranches(t *testing.T) {
	d, _ := ParseDate("2024-01-01")
	if _, ok := Member(d, "nope"); ok {
		t.Error("unknown date member should be ok=false")
	}
	tm := NewTime(1, 2, 3, 0, nil)
	if _, ok := Member(tm, "nope"); ok {
		t.Error("unknown time member should be ok=false")
	}
	if s, ok := Member(NewDaysTimeDuration(time.Hour+90*time.Second), "seconds"); !ok || s.String() != "30" {
		t.Errorf("dtd seconds = %v ok=%v, want 30", s, ok)
	}
	if _, ok := Member(NewDaysTimeDuration(0), "nope"); ok {
		t.Error("unknown dtd member should be ok=false")
	}
	if _, ok := Member(NewYearsMonthsDuration(0), "nope"); ok {
		t.Error("unknown ymd member should be ok=false")
	}
	// Sunday maps to ISO weekday 7.
	sun, _ := ParseDate("2024-03-03")
	if w, ok := Member(sun, "weekday"); !ok || w.String() != "7" {
		t.Errorf("Sunday weekday = %v, want 7", w)
	}
	// A zoned Time exposes its offset.
	off := NewDaysTimeDuration(2 * time.Hour)
	zt := NewTime(1, 0, 0, 0, &off)
	if o, ok := Member(zt, "time offset"); !ok || o.Kind() != KindDaysTimeDuration {
		t.Errorf("zoned time offset = %v ok=%v", o, ok)
	}
	// A zoned-by-offset (no IANA name) Time has a null timezone name.
	if tz, ok := Member(zt, "timezone"); !ok || !IsNull(tz) {
		t.Errorf("offset-only timezone = %v, want null", tz)
	}
}
