package builtins

import (
	"time"

	"github.com/pblumer/temis/internal/value"
)

// nowFunc supplies the current instant for now() and today(). It is a package
// variable so tests can substitute a fixed clock; production code never mutates
// it after start-up, which keeps evaluation deterministic and goroutine-safe.
var nowFunc = time.Now

func registerTemporal(r *Registry) {
	// date(from) | date(year, month, day). Both named forms bind: date(from:…)
	// via the primary signature, date(year:…, month:…, day:…) via the alternate.
	r.Register(overloaded("date", []string{"from"}, [][]string{{"year", "month", "day"}}, 1, 3, dateFn))
	// time(from) | time(hour, minute, second) | time(hour, minute, second, offset).
	r.Register(overloaded("time", []string{"from"}, [][]string{{"hour", "minute", "second", "offset"}}, 1, 4, timeFn))
	// date and time(from) | date and time(date, time).
	r.Register(overloaded("date and time", []string{"from"}, [][]string{{"date", "time"}}, 1, 2, dateAndTimeFn))
	// duration(from): parse an ISO-8601 duration string (or pass a duration through).
	r.Register(fixed("duration", []string{"from"}, 1, 1, durationFn))
	// years and months duration(from, to): whole-month difference between dates.
	r.Register(fixed("years and months duration", []string{"from", "to"}, 2, 2, yearsMonthsFn))

	// now(): current date and time. today(): current date.
	r.Register(fixed("now", nil, 0, 0, func([]value.Value) (value.Value, error) {
		return value.NewDateTime(nowFunc()), nil
	}))
	r.Register(fixed("today", nil, 0, 0, func([]value.Value) (value.Value, error) {
		t := nowFunc()
		return value.DateFromComponents(t.Year(), int(t.Month()), t.Day()), nil
	}))

	// Calendar accessors on a date (or date-and-time).
	r.Register(fixed("day of week", []string{"date"}, 1, 1, dateString(func(t time.Time) string { return t.Weekday().String() })))
	r.Register(fixed("month of year", []string{"date"}, 1, 1, dateString(func(t time.Time) string { return t.Month().String() })))
	r.Register(fixed("day of year", []string{"date"}, 1, 1, dateNumber(func(t time.Time) int64 { return int64(t.YearDay()) })))
	r.Register(fixed("week of year", []string{"date"}, 1, 1, dateNumber(func(t time.Time) int64 {
		_, w := t.ISOWeek()
		return int64(w)
	})))
}

func dateFn(args []value.Value) (value.Value, error) {
	switch len(args) {
	case 1:
		switch v := args[0].(type) {
		case value.Date:
			return v, nil
		case value.Str:
			if d, err := value.ParseDate(string(v)); err == nil {
				return d, nil
			}
			return value.Null, nil
		case value.DateTime:
			t := v.Time()
			return value.DateFromComponents(t.Year(), int(t.Month()), t.Day()), nil
		default:
			return value.Null, nil
		}
	case 3:
		y, ok1 := asInt(args[0])
		m, ok2 := asInt(args[1])
		d, ok3 := asInt(args[2])
		if !ok1 || !ok2 || !ok3 || !validYMD(y, m, d) {
			return value.Null, nil
		}
		return value.DateFromComponents(y, m, d), nil
	default:
		return value.Null, nil
	}
}

// validYMD rejects out-of-range month/day combinations (e.g. Feb 30) by
// confirming the components survive a round-trip through a calendar date.
func validYMD(y, m, d int) bool {
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return false
	}
	if y < -999999999 || y > 999999999 { // FEEL year magnitude bound
		return false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t.Year() == y && int(t.Month()) == m && t.Day() == d
}

func timeFn(args []value.Value) (value.Value, error) {
	switch len(args) {
	case 1:
		switch v := args[0].(type) {
		case value.Time:
			return v, nil
		case value.Str:
			if t, err := value.ParseTime(string(v)); err == nil {
				return t, nil
			}
			return value.Null, nil
		case value.DateTime:
			return value.TimeOf(v), nil
		case value.Date:
			// A plain date has no time-of-day: time(date) is midnight at UTC (TCK 1116).
			z := value.NewDaysTimeDuration(0)
			return value.NewTime(0, 0, 0, 0, &z), nil
		default:
			return value.Null, nil
		}
	case 3, 4:
		h, ok1 := asInt(args[0])
		m, ok2 := asInt(args[1])
		// The seconds component may be fractional (e.g. 1.3), yielding sub-second
		// precision on the resulting time.
		s, nanos, ok3 := asSecond(args[2])
		if !ok1 || !ok2 || !ok3 || !validHMS(h, m, s) {
			return value.Null, nil
		}
		var offset *value.DaysTimeDuration
		if len(args) == 4 && !value.IsNull(args[3]) {
			d, ok := args[3].(value.DaysTimeDuration)
			if !ok {
				return value.Null, nil
			}
			offset = &d
		}
		return value.NewTime(h, m, s, nanos, offset), nil
	default:
		return value.Null, nil
	}
}

func validHMS(h, m, s int) bool {
	return h >= 0 && h < 24 && m >= 0 && m < 60 && s >= 0 && s < 60
}

func dateAndTimeFn(args []value.Value) (value.Value, error) {
	switch len(args) {
	case 1:
		switch v := args[0].(type) {
		case value.DateTime:
			return v, nil
		case value.Date:
			// a date becomes date-and-time at the start of its day
			if dt, err := value.ParseDateTime(v.String() + "T00:00:00"); err == nil {
				return dt, nil
			}
			return value.Null, nil
		case value.Str:
			if dt, err := value.ParseDateTime(string(v)); err == nil {
				return dt, nil
			}
			// a date-only string yields date-and-time at the start of the day
			if _, err := value.ParseDate(string(v)); err == nil {
				if dt, err := value.ParseDateTime(string(v) + "T00:00:00"); err == nil {
					return dt, nil
				}
			}
			return value.Null, nil
		default:
			return value.Null, nil
		}
	case 2:
		// The first argument may be a date or a date-and-time (its date part is
		// taken); the second is the time to attach.
		d, ok1 := dateValue(args[0])
		t, ok2 := args[1].(value.Time)
		if !ok1 || !ok2 {
			return value.Null, nil
		}
		return value.CombineDateTime(d, t), nil
	default:
		return value.Null, nil
	}
}

func durationFn(args []value.Value) (value.Value, error) {
	switch v := args[0].(type) {
	case value.DaysTimeDuration, value.YearsMonthsDuration:
		return v, nil
	case value.Str:
		if d, err := value.ParseDuration(string(v)); err == nil {
			return d, nil
		}
		return value.Null, nil
	default:
		return value.Null, nil
	}
}

func yearsMonthsFn(args []value.Value) (value.Value, error) {
	from, ok1 := dateValue(args[0])
	to, ok2 := dateValue(args[1])
	if !ok1 || !ok2 {
		return value.Null, nil
	}
	return value.YearsMonthsBetween(from, to), nil
}

// dateValue coerces a date or date-and-time argument to a Date.
func dateValue(v value.Value) (value.Date, bool) {
	switch x := v.(type) {
	case value.Date:
		return x, true
	case value.DateTime:
		t := x.Time()
		return value.DateFromComponents(t.Year(), int(t.Month()), t.Day()), true
	default:
		return value.Date{}, false
	}
}

// dateInstant extracts the backing calendar instant from a date or date-and-time
// argument (parsing a string as a date), for the calendar-accessor builtins.
func dateInstant(v value.Value) (time.Time, bool) {
	switch x := v.(type) {
	case value.Date:
		return x.Time(), true
	case value.DateTime:
		return x.Time(), true
	case value.Str:
		if d, err := value.ParseDate(string(x)); err == nil {
			return d.Time(), true
		}
	}
	return time.Time{}, false
}

func dateString(f func(time.Time) string) Func {
	return func(args []value.Value) (value.Value, error) {
		t, ok := dateInstant(args[0])
		if !ok {
			return value.Null, nil
		}
		return value.Str(f(t)), nil
	}
}

func dateNumber(f func(time.Time) int64) Func {
	return func(args []value.Value) (value.Value, error) {
		t, ok := dateInstant(args[0])
		if !ok {
			return value.Null, nil
		}
		return value.NumberFromInt64(f(t)), nil
	}
}
