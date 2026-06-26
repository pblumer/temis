package value

import "time"

// This file backs the DMN 1.5 temporal built-ins (WP-22): exported constructors
// for time/date-and-time/durations from components, the difference between two
// dates as a years-and-months duration, and property access (Member) used by
// FEEL path navigation such as date("2024-01-31").year.

// NewTime builds a time-of-day from components. When offset is non-nil the time
// is zoned at that fixed UTC offset; otherwise it is a local time without a zone.
func NewTime(hour, minute, second, nanos int, offset *DaysTimeDuration) Time {
	loc := time.UTC
	z := zone{zoned: false}
	if offset != nil {
		secs := int(offset.d / time.Second)
		loc = time.FixedZone("", secs)
		z = zone{zoned: true}
	}
	t := time.Date(1, 1, 1, hour, minute, second, nanos, loc)
	return Time{t: t, z: z}
}

// CombineDateTime merges a date with a time-of-day into a date-and-time, taking
// the zone from the time component.
func CombineDateTime(d Date, t Time) DateTime {
	loc := t.t.Location()
	combined := time.Date(d.t.Year(), d.t.Month(), d.t.Day(),
		t.t.Hour(), t.t.Minute(), t.t.Second(), t.t.Nanosecond(), loc)
	return DateTime{t: combined, z: t.z}
}

// DateFromComponents builds a Date from year, month and day.
func DateFromComponents(year, month, day int) Date {
	return NewDate(year, time.Month(month), day)
}

// TimeOf extracts the time-of-day (with zone) from a date-and-time. Time and
// DateTime share an identical layout, so a direct conversion suffices.
func TimeOf(dt DateTime) Time {
	return Time(dt)
}

// NewDaysTimeDuration builds a days-and-time duration from a time.Duration.
func NewDaysTimeDuration(d time.Duration) DaysTimeDuration { return DaysTimeDuration{d: d} }

// NewYearsMonthsDuration builds a years-and-months duration from a month count.
func NewYearsMonthsDuration(months int64) YearsMonthsDuration {
	return YearsMonthsDuration{months: months}
}

// YearsMonthsBetween returns the difference from→to as a whole-month duration,
// truncated toward zero (e.g. 2020-01-01 → 2021-06-15 is P1Y5M).
func YearsMonthsBetween(from, to Date) YearsMonthsDuration {
	if to.t.Before(from.t) {
		d := YearsMonthsBetween(to, from)
		return YearsMonthsDuration{months: -d.months}
	}
	total := int64((to.t.Year()-from.t.Year())*12 + int(to.t.Month()) - int(from.t.Month()))
	if to.t.Day() < from.t.Day() {
		total--
	}
	return YearsMonthsDuration{months: total}
}

// Member returns the named FEEL property of a temporal or duration value and
// reports whether v has such a property. It backs path access for these types;
// non-temporal values return ok=false so the caller can fall back to context
// member lookup.
func Member(v Value, name string) (Value, bool) {
	switch x := v.(type) {
	case Date:
		return dateMember(x.t, name)
	case DateTime:
		return dateTimeMember(x.t, x.z, name)
	case Time:
		return timeMember(x.t, x.z, name)
	case DaysTimeDuration:
		return dtDurationMember(x.d, name)
	case YearsMonthsDuration:
		return ymDurationMember(x.months, name)
	default:
		return nil, false
	}
}

func numMember(i int64) (Value, bool) { return NumberFromInt64(i), true }

// isoWeekday maps Go's Sunday=0..Saturday=6 onto FEEL's Monday=1..Sunday=7.
func isoWeekday(t time.Time) int64 {
	wd := int(t.Weekday())
	if wd == 0 {
		return 7
	}
	return int64(wd)
}

func dateMember(t time.Time, name string) (Value, bool) {
	switch name {
	case "year":
		return numMember(int64(t.Year()))
	case "month":
		return numMember(int64(t.Month()))
	case "day":
		return numMember(int64(t.Day()))
	case "weekday":
		return numMember(isoWeekday(t))
	default:
		return nil, false
	}
}

func timeMember(t time.Time, z zone, name string) (Value, bool) {
	switch name {
	case "hour":
		return numMember(int64(t.Hour()))
	case "minute":
		return numMember(int64(t.Minute()))
	case "second":
		return numMember(int64(t.Second()))
	case "time offset", "timezone":
		return zoneMember(t, z, name)
	default:
		return nil, false
	}
}

func dateTimeMember(t time.Time, z zone, name string) (Value, bool) {
	if v, ok := dateMember(t, name); ok {
		return v, ok
	}
	return timeMember(t, z, name)
}

// zoneMember returns the "time offset" (a days-and-time duration) or "timezone"
// (the IANA name string) property, or null when the value carries no zone.
func zoneMember(t time.Time, z zone, name string) (Value, bool) {
	if !z.zoned {
		return Null, true
	}
	if name == "time offset" {
		_, secs := t.Zone()
		return DaysTimeDuration{d: time.Duration(secs) * time.Second}, true
	}
	// timezone: the IANA name when given via @Area/City, else null.
	if z.name != "" {
		return Str(z.name), true
	}
	return Null, true
}

func dtDurationMember(d time.Duration, name string) (Value, bool) {
	switch name {
	case "days":
		return numMember(int64(d / (24 * time.Hour)))
	case "hours":
		return numMember(int64((d % (24 * time.Hour)) / time.Hour))
	case "minutes":
		return numMember(int64((d % time.Hour) / time.Minute))
	case "seconds":
		return numMember(int64((d % time.Minute) / time.Second))
	default:
		return nil, false
	}
}

func ymDurationMember(months int64, name string) (Value, bool) {
	switch name {
	case "years":
		return numMember(months / 12)
	case "months":
		return numMember(months % 12)
	default:
		return nil, false
	}
}
