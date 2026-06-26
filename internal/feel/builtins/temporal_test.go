package builtins

import (
	"testing"
	"time"

	"github.com/pblumer/temis/internal/value"
)

func TestTemporalConstructors(t *testing.T) {
	run(t, []tc{
		// date
		{name: "date", args: []value.Value{str("2024-02-29")}, want: "2024-02-29"},
		{name: "date", args: []value.Value{num("2024"), num("2"), num("29")}, want: "2024-02-29"},
		{name: "date", args: []value.Value{num("2023"), num("2"), num("29")}, wantNull: true}, // not a leap year
		{name: "date", args: []value.Value{num("2024"), num("13"), num("1")}, wantNull: true},
		{name: "date", args: []value.Value{str("nope")}, wantNull: true},

		// time
		{name: "time", args: []value.Value{num("13"), num("30"), num("0")}, want: "13:30:00"},
		{name: "time", args: []value.Value{str("08:00:00")}, want: "08:00:00"},
		{name: "time", args: []value.Value{num("25"), num("0"), num("0")}, wantNull: true},

		// date and time from string and from (date, time)
		{name: "date and time", args: []value.Value{str("2024-02-29T13:30:00")}, want: "2024-02-29T13:30:00"},

		// duration
		{name: "duration", args: []value.Value{str("P1Y6M")}, want: "P1Y6M"},
		{name: "duration", args: []value.Value{str("PT2H30M")}, want: "P0DT2H30M0S"},
		{name: "duration", args: []value.Value{str("bogus")}, wantNull: true},

		// calendar accessors (2024-02-29 is a Thursday, day 60, ISO week 9)
		{name: "day of week", args: []value.Value{str("2024-02-29")}, want: "Thursday"},
		{name: "month of year", args: []value.Value{str("2024-02-29")}, want: "February"},
		{name: "day of year", args: []value.Value{str("2024-02-29")}, want: "60"},
		{name: "week of year", args: []value.Value{str("2024-02-29")}, want: "9"},

		// years and months duration
		{name: "years and months duration", args: []value.Value{mustDate("2020-01-01"), mustDate("2021-06-15")}, want: "P1Y5M"},
	})
}

func TestDateAndTimeFromParts(t *testing.T) {
	d := mustDate("2024-02-29")
	tm := value.NewTime(13, 30, 0, 0, nil)
	got := call(t, "date and time", d, tm)
	if got.String() != "2024-02-29T13:30:00" {
		t.Errorf("date and time(date, time) = %s, want 2024-02-29T13:30:00", got)
	}
}

func TestNowToday(t *testing.T) {
	fixed := time.Date(2024, 6, 26, 9, 15, 30, 0, time.UTC)
	orig := nowFunc
	nowFunc = func() time.Time { return fixed }
	defer func() { nowFunc = orig }()

	if got := call(t, "now"); got.String() != "2024-06-26T09:15:30Z" {
		t.Errorf("now() = %s, want 2024-06-26T09:15:30Z", got)
	}
	if got := call(t, "today"); got.String() != "2024-06-26" {
		t.Errorf("today() = %s, want 2024-06-26", got)
	}
}

func TestTemporalCoercionsAndNulls(t *testing.T) {
	dt, _ := value.ParseDateTime("2024-02-29T13:30:45+01:00")
	d := mustDate("2024-02-29")
	tmLocal := value.NewTime(8, 0, 0, 0, nil)
	off := value.NewDaysTimeDuration(2 * time.Hour)

	run(t, []tc{
		// date(...) passthrough and extraction from date-and-time
		{name: "date", args: []value.Value{d}, want: "2024-02-29"},
		{name: "date", args: []value.Value{dt}, want: "2024-02-29"},
		{name: "date", args: []value.Value{num("1"), num("2")}, wantNull: true}, // invalid arity → null
		{name: "date", args: []value.Value{value.True}, wantNull: true},

		// time(...) passthrough, extraction, and zoned construction
		{name: "time", args: []value.Value{tmLocal}, want: "08:00:00"},
		{name: "time", args: []value.Value{dt}, want: "13:30:45+01:00"},
		{name: "time", args: []value.Value{num("13"), num("30"), num("0"), off}, want: "13:30:00+02:00"},
		{name: "time", args: []value.Value{num("13"), num("30"), num("0"), str("x")}, wantNull: true},
		{name: "time", args: []value.Value{value.True}, wantNull: true},

		// date and time passthrough / bad string / wrong types
		{name: "date and time", args: []value.Value{dt}, want: "2024-02-29T13:30:45+01:00"},
		{name: "date and time", args: []value.Value{str("nope")}, wantNull: true},
		{name: "date and time", args: []value.Value{d, str("x")}, wantNull: true},

		// duration passthrough
		{name: "duration", args: []value.Value{off}, want: "P0DT2H0M0S"},
		{name: "duration", args: []value.Value{num("5")}, wantNull: true},

		// years and months duration accepts date-and-time, rejects non-dates
		{name: "years and months duration", args: []value.Value{dt, dt}, want: "P0Y0M"},
		{name: "years and months duration", args: []value.Value{str("x"), d}, wantNull: true},

		// calendar accessors: from date-and-time and null on bad input
		{name: "day of week", args: []value.Value{dt}, want: "Thursday"},
		{name: "day of year", args: []value.Value{num("5")}, wantNull: true},
	})
}

func mustDate(s string) value.Value {
	d, err := value.ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}
