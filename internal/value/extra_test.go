package value

import (
	"testing"
	"time"
)

func TestKindStringAll(t *testing.T) {
	kinds := []Kind{
		KindNull, KindBool, KindNumber, KindString, KindDate, KindTime, KindDateTime,
		KindDaysTimeDuration, KindYearsMonthsDuration, KindList, KindContext, KindRange,
		KindFunction, Kind(250),
	}
	for _, k := range kinds {
		if k.String() == "" {
			t.Errorf("Kind(%d).String() empty", k)
		}
	}
	if Kind(250).String() != "unknown" {
		t.Errorf("unknown kind = %q", Kind(250).String())
	}
}

func TestNumberAccessors(t *testing.T) {
	n := MustNumber("0")
	if !n.IsZero() {
		t.Error("0.IsZero() should be true")
	}
	if MustNumber("5").IsZero() {
		t.Error("5.IsZero() should be false")
	}
	if MustNumber("42").Decimal().Text('f') != "42" {
		t.Error("Decimal() accessor wrong")
	}
}

func TestFunctionValue(t *testing.T) {
	f := &Function{Name: "sum", Arity: 2, Call: func(args []Value) (Value, error) { return args[0], nil }}
	if f.Kind() != KindFunction || f.String() != "function sum" {
		t.Errorf("function = %s/%s", f.String(), f.Kind())
	}
	anon := &Function{}
	if anon.String() != "function" {
		t.Errorf("anon function = %q", anon.String())
	}
	if Equal(f, f) != False { // functions are never equal
		t.Error("functions should compare unequal")
	}
}

func TestSubDurationsAndTemporals(t *testing.T) {
	ymA, _ := ParseDuration("P2Y")
	ymB, _ := ParseDuration("P6M")
	if got := Sub(ymA, ymB).(YearsMonthsDuration).Months(); got != 18 {
		t.Errorf("P2Y - P6M = %d months, want 18", got)
	}
	dtA, _ := ParseDuration("PT5H")
	dtB, _ := ParseDuration("PT2H")
	if got := Sub(dtA, dtB).(DaysTimeDuration).Duration(); got != 3*time.Hour {
		t.Errorf("PT5H - PT2H = %v, want 3h", got)
	}
	if got := Add(dtA, dtB).(DaysTimeDuration).Duration(); got != 7*time.Hour {
		t.Errorf("PT5H + PT2H = %v, want 7h", got)
	}
	if got := Add(ymA, ymB).(YearsMonthsDuration).Months(); got != 30 {
		t.Errorf("P2Y + P6M = %d, want 30", got)
	}

	tA, _ := ParseTime("12:00:00")
	tB, _ := ParseTime("10:00:00")
	if got := Sub(tA, tB).(DaysTimeDuration).Duration(); got != 2*time.Hour {
		t.Errorf("time diff = %v, want 2h", got)
	}
	dtX, _ := ParseDateTime("2024-01-02T00:00:00")
	dtY, _ := ParseDateTime("2024-01-01T00:00:00")
	if got := Sub(dtX, dtY).(DaysTimeDuration).Duration(); got != 24*time.Hour {
		t.Errorf("datetime diff = %v, want 24h", got)
	}
}

func TestTemporalShiftVariants(t *testing.T) {
	// date - years-months
	d := NewDate(2024, time.March, 15)
	ym, _ := ParseDuration("P1M")
	if got := Sub(d, ym).String(); got != "2024-02-15" {
		t.Errorf("2024-03-15 - P1M = %s, want 2024-02-15", got)
	}
	// datetime + years-months
	dt, _ := ParseDateTime("2024-01-15T08:00:00")
	if got := Add(dt, ym).String(); got != "2024-02-15T08:00:00" {
		t.Errorf("datetime + P1M = %s", got)
	}
	// time + days-time
	tm, _ := ParseTime("23:00:00")
	dur, _ := ParseDuration("PT2H")
	if got := Add(tm, dur).String(); got != "01:00:00" {
		t.Errorf("23:00 + PT2H = %s, want 01:00:00", got)
	}
	// duration + date (commuted)
	if got := Add(ym, d).String(); got != "2024-04-15" {
		t.Errorf("P1M + 2024-03-15 = %s, want 2024-04-15", got)
	}
}

func TestMulCommutativeAndDivDurations(t *testing.T) {
	dt, _ := ParseDuration("PT1H")
	// number * duration (number first)
	if got := Mul(MustNumber("3"), dt).(DaysTimeDuration).Duration(); got != 3*time.Hour {
		t.Errorf("3 * PT1H = %v, want 3h", got)
	}
	// dtd / number
	if got := Div(dt, MustNumber("2")).(DaysTimeDuration).Duration(); got != 30*time.Minute {
		t.Errorf("PT1H / 2 = %v, want 30m", got)
	}
	// ymd / number
	ym, _ := ParseDuration("P2Y")
	if got := Div(ym, MustNumber("2")).(YearsMonthsDuration).Months(); got != 12 {
		t.Errorf("P2Y / 2 = %d, want 12", got)
	}
	// ymd / ymd → number
	ymB, _ := ParseDuration("P1Y")
	if got := Div(ym, ymB); IsNull(got) || got.String() != "2" {
		t.Errorf("P2Y / P1Y = %s, want 2", got)
	}
	// division by zero number → null
	if !IsNull(Div(dt, MustNumber("0"))) {
		t.Error("PT1H / 0 should be null")
	}
}

func TestNegAndExpEdges(t *testing.T) {
	dt, _ := ParseDuration("PT1H")
	if got := Neg(dt).(DaysTimeDuration).Duration(); got != -time.Hour {
		t.Errorf("-PT1H = %v", got)
	}
	ym, _ := ParseDuration("P1Y")
	if got := Neg(ym).(YearsMonthsDuration).Months(); got != -12 {
		t.Errorf("-P1Y = %d", got)
	}
	if !IsNull(Neg(Str("x"))) {
		t.Error("-string should be null")
	}
	if !IsNull(Exp(Str("x"), MustNumber("2"))) || !IsNull(Exp(MustNumber("2"), Str("x"))) {
		t.Error("** with non-number should be null")
	}
	// FEEL: string + string concatenates
	if got := Add(Str("a"), Str("b")); got != Str("ab") {
		t.Errorf(`"a" + "b" = %q, want "ab"`, got)
	}
	// undefined combinations
	if !IsNull(Div(Str("a"), MustNumber("1"))) {
		t.Error("string / number should be null")
	}
}

func TestRangeEqualAndDurationCompare(t *testing.T) {
	r1 := Range{LowClosed: true, Low: MustNumber("1"), High: MustNumber("10"), HighClosed: false}
	r2 := Range{LowClosed: true, Low: MustNumber("1"), High: MustNumber("10"), HighClosed: false}
	r3 := Range{LowClosed: false, Low: MustNumber("1"), High: MustNumber("10"), HighClosed: false}
	if Equal(r1, r2) != True {
		t.Error("identical ranges should be equal")
	}
	if Equal(r1, r3) != False {
		t.Error("ranges with different bounds should be unequal")
	}

	a, _ := ParseDuration("PT2H")
	b, _ := ParseDuration("PT1H")
	if c, ok := Compare(a, b); !ok || c != 1 {
		t.Errorf("PT2H cmp PT1H = (%d,%v), want (1,true)", c, ok)
	}
	ymA, _ := ParseDuration("P1Y")
	ymB, _ := ParseDuration("P2Y")
	if c, ok := Compare(ymA, ymB); !ok || c != -1 {
		t.Errorf("P1Y cmp P2Y = (%d,%v), want (-1,true)", c, ok)
	}
}

func TestDateTimeAccessorAndError(t *testing.T) {
	dt, _ := ParseDateTime("2024-01-01T00:00:00")
	if dt.Time().Year() != 2024 {
		t.Error("DateTime.Time() accessor wrong")
	}
	if _, err := ParseDate("not-a-date"); err == nil {
		t.Error("ParseDate(garbage) should error")
	}
	if _, err := ParseDateTime("2024-01-01"); err == nil {
		t.Error("ParseDateTime without T should error")
	}
	if _, err := ParseTime("99:99:99"); err == nil {
		t.Error("ParseTime(invalid) should error")
	}
	if _, err := ParseTime("12:00:00@Nowhere/Nope"); err == nil {
		t.Error("ParseTime with unknown zone should error")
	}
}
