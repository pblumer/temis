package value

import (
	"testing"
	"time"
)

func TestNewTimeAndCombine(t *testing.T) {
	// local time (no zone)
	lt := NewTime(13, 30, 0, 0, nil)
	if lt.String() != "13:30:00" {
		t.Errorf("local time = %q, want 13:30:00", lt.String())
	}
	// zoned time at +02:00
	off := NewDaysTimeDuration(2 * time.Hour)
	zt := NewTime(13, 30, 0, 0, &off)
	if zt.String() != "13:30:00+02:00" {
		t.Errorf("zoned time = %q, want 13:30:00+02:00", zt.String())
	}
	// combine date + time
	d := DateFromComponents(2024, 1, 31)
	dt := CombineDateTime(d, zt)
	if dt.String() != "2024-01-31T13:30:00+02:00" {
		t.Errorf("combined = %q, want 2024-01-31T13:30:00+02:00", dt.String())
	}
}

func TestYearsMonthsBetween(t *testing.T) {
	cases := []struct {
		from, to string
		want     string
	}{
		{"2020-01-01", "2021-06-15", "P1Y5M"},
		{"2020-01-15", "2021-06-01", "P1Y4M"},
		{"2021-06-15", "2020-01-01", "-P1Y5M"},
		{"2024-01-01", "2024-01-01", "P0Y0M"},
	}
	for _, c := range cases {
		from, _ := ParseDate(c.from)
		to, _ := ParseDate(c.to)
		got := YearsMonthsBetween(from, to)
		if got.String() != c.want {
			t.Errorf("between(%s,%s) = %s, want %s", c.from, c.to, got, c.want)
		}
	}
}

func TestMemberAccess(t *testing.T) {
	d, _ := ParseDate("2024-02-29") // a Thursday
	check := func(v Value, name, want string) {
		t.Helper()
		got, ok := Member(v, name)
		if !ok || got.String() != want {
			t.Errorf("Member(%s).%s = %v ok=%v, want %s", v, name, got, ok, want)
		}
	}
	check(d, "year", "2024")
	check(d, "month", "2")
	check(d, "day", "29")
	check(d, "weekday", "4") // Thursday

	dt, _ := ParseDateTime("2024-02-29T13:30:45@Europe/Paris")
	check(dt, "hour", "13")
	check(dt, "minute", "30")
	check(dt, "second", "45")
	check(dt, "year", "2024")
	check(dt, "timezone", "Europe/Paris")
	if off, ok := Member(dt, "time offset"); !ok || off.Kind() != KindDaysTimeDuration {
		t.Errorf("time offset = %v ok=%v, want a duration", off, ok)
	}

	// local time has null offset/timezone
	lt := NewTime(8, 0, 0, 0, nil)
	if off, ok := Member(lt, "time offset"); !ok || !IsNull(off) {
		t.Errorf("local time offset = %v, want null", off)
	}

	ym := NewYearsMonthsDuration(17)
	check(ym, "years", "1")
	check(ym, "months", "5")

	dtd := NewDaysTimeDuration(26*time.Hour + 90*time.Minute)
	check(dtd, "days", "1")
	check(dtd, "hours", "3")
	check(dtd, "minutes", "30")

	// non-temporal → not a member
	if _, ok := Member(Str("x"), "year"); ok {
		t.Error("Member on string should report ok=false")
	}
}
