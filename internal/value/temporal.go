package value

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// --- Date ---

// Date is a calendar date with no time-of-day or zone. It is stored as midnight
// UTC for the given day.
type Date struct {
	t time.Time
}

// Kind returns KindDate.
func (Date) Kind() Kind       { return KindDate }
func (Date) isValue()         {}
func (d Date) String() string { return d.t.Format("2006-01-02") }

// Time returns the underlying instant (midnight UTC).
func (d Date) Time() time.Time { return d.t }

// NewDate builds a Date from year, month, day.
func NewDate(year int, month time.Month, day int) Date {
	return Date{t: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate parses an ISO date "YYYY-MM-DD".
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: %w", s, err)
	}
	return Date{t: t.UTC()}, nil
}

// --- Time and DateTime share zone handling ---

// zone describes the timezone attached to a time or date-and-time value.
type zone struct {
	zoned bool   // false ⇒ local time without zone
	name  string // IANA name when given via @Area/City, else empty
}

func (z zone) suffix(t time.Time) string {
	if !z.zoned {
		return ""
	}
	if z.name != "" {
		return "@" + z.name
	}
	if off := t.Format("-07:00"); off == "+00:00" {
		return "Z"
	} else {
		return off
	}
}

// Time is a time-of-day, optionally with a zone. The date part of the backing
// instant is a fixed reference day and is not significant.
type Time struct {
	t time.Time
	z zone
}

// Kind returns KindTime.
func (Time) Kind() Kind { return KindTime }
func (Time) isValue()   {}
func (t Time) String() string {
	return t.t.Format("15:04:05") + t.z.suffix(t.t)
}

// DateTime is a date and time, optionally with a zone.
type DateTime struct {
	t time.Time
	z zone
}

// Kind returns KindDateTime.
func (DateTime) Kind() Kind { return KindDateTime }
func (DateTime) isValue()   {}
func (dt DateTime) String() string {
	return dt.t.Format("2006-01-02T15:04:05") + dt.z.suffix(dt.t)
}

// Time returns the underlying instant.
func (dt DateTime) Time() time.Time { return dt.t }

// NewDateTime returns a zoned DateTime for the instant t. The location of t
// determines the rendered zone suffix (UTC renders as "Z"). It is the entry
// point used when converting a Go time.Time input into a FEEL value.
func NewDateTime(t time.Time) DateTime {
	return DateTime{t: t, z: zone{zoned: true}}
}

const refDay = "0001-01-01"

// ParseTime parses "HH:MM:SS(.fff)?" with an optional Z, ±HH:MM or @Zone suffix.
func ParseTime(s string) (Time, error) {
	body, z, loc, err := splitZone(s)
	if err != nil {
		return Time{}, fmt.Errorf("invalid time %q: %w", s, err)
	}
	t, err := parseClock(refDay+"T"+body, loc)
	if err != nil {
		return Time{}, fmt.Errorf("invalid time %q: %w", s, err)
	}
	return Time{t: t, z: z}, nil
}

// ParseDateTime parses "YYYY-MM-DDTHH:MM:SS(.fff)?" with an optional zone suffix.
func ParseDateTime(s string) (DateTime, error) {
	i := strings.IndexByte(s, 'T')
	if i < 0 {
		return DateTime{}, fmt.Errorf("invalid date and time %q: missing 'T'", s)
	}
	body, z, loc, err := splitZone(s)
	if err != nil {
		return DateTime{}, fmt.Errorf("invalid date and time %q: %w", s, err)
	}
	t, err := parseClock(body, loc)
	if err != nil {
		return DateTime{}, fmt.Errorf("invalid date and time %q: %w", s, err)
	}
	return DateTime{t: t, z: z}, nil
}

// parseClock parses a "YYYY-MM-DDTHH:MM:SS(.fff)?" body in the given location,
// trying with and without fractional seconds.
func parseClock(body string, loc *time.Location) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, body, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("malformed clock %q", body)
}

// splitZone separates an optional trailing zone designator from a time or
// date-and-time string and resolves it to a location.
func splitZone(s string) (body string, z zone, loc *time.Location, err error) {
	if at := strings.LastIndexByte(s, '@'); at >= 0 {
		name := s[at+1:]
		l, e := time.LoadLocation(name)
		if e != nil {
			return "", zone{}, nil, fmt.Errorf("unknown zone %q: %w", name, e)
		}
		return s[:at], zone{zoned: true, name: name}, l, nil
	}
	if strings.HasSuffix(s, "Z") {
		return strings.TrimSuffix(s, "Z"), zone{zoned: true}, time.UTC, nil
	}
	// Offset ±HH:MM appears after the clock; find the sign that is not the date's.
	if off, idx := findOffset(s); idx >= 0 {
		d, e := parseOffset(off)
		if e != nil {
			return "", zone{}, nil, e
		}
		return s[:idx], zone{zoned: true}, d, nil
	}
	return s, zone{zoned: false}, time.UTC, nil
}

// findOffset locates a trailing ±HH:MM offset in a time/date-time string,
// ignoring the '-' separators of the date part.
func findOffset(s string) (string, int) {
	if len(s) < 6 {
		return "", -1
	}
	tail := s[len(s)-6:]
	if (tail[0] == '+' || tail[0] == '-') && tail[3] == ':' {
		return tail, len(s) - 6
	}
	return "", -1
}

func parseOffset(off string) (*time.Location, error) {
	t, err := time.Parse("-07:00", off)
	if err != nil {
		return nil, fmt.Errorf("invalid offset %q: %w", off, err)
	}
	_, secs := t.Zone()
	return time.FixedZone(off, secs), nil
}

// --- Durations ---

// DaysTimeDuration is a duration measured in seconds/nanoseconds (may be
// negative). It cannot be converted to a YearsMonthsDuration.
type DaysTimeDuration struct {
	d time.Duration
}

// Kind returns KindDaysTimeDuration.
func (DaysTimeDuration) Kind() Kind { return KindDaysTimeDuration }
func (DaysTimeDuration) isValue()   {}

// Duration returns the duration as a time.Duration.
func (d DaysTimeDuration) Duration() time.Duration { return d.d }

func (d DaysTimeDuration) String() string {
	dur := d.d
	sign := ""
	if dur < 0 {
		sign = "-"
		dur = -dur
	}
	days := dur / (24 * time.Hour)
	rem := dur % (24 * time.Hour)
	h := rem / time.Hour
	m := (rem % time.Hour) / time.Minute
	sec := (rem % time.Minute) / time.Second
	var b strings.Builder
	fmt.Fprintf(&b, "%sP%dDT%dH%dM%dS", sign, days, h, m, sec)
	return b.String()
}

// YearsMonthsDuration is a duration measured in whole months (may be negative).
type YearsMonthsDuration struct {
	months int64
}

// Kind returns KindYearsMonthsDuration.
func (YearsMonthsDuration) Kind() Kind { return KindYearsMonthsDuration }
func (YearsMonthsDuration) isValue()   {}

// Months returns the total number of months (may be negative).
func (d YearsMonthsDuration) Months() int64 { return d.months }

func (d YearsMonthsDuration) String() string {
	m := d.months
	sign := ""
	if m < 0 {
		sign = "-"
		m = -m
	}
	return fmt.Sprintf("%sP%dY%dM", sign, m/12, m%12)
}

// ParseDuration parses an ISO 8601 duration into either a years-months or a
// days-time duration. The two FEEL duration types are disjoint, so a literal
// mixing year/month and day/time components is rejected. Fractional components
// are not accepted (refined in WP-22).
func ParseDuration(s string) (Value, error) {
	neg := false
	body := s
	if strings.HasPrefix(body, "-") {
		neg = true
		body = body[1:]
	}
	if !strings.HasPrefix(body, "P") {
		return nil, fmt.Errorf("invalid duration %q: must start with P", s)
	}
	body = body[1:]

	datePart, timePart := body, ""
	hadT := false
	if i := strings.IndexByte(body, 'T'); i >= 0 {
		datePart, timePart, hadT = body[:i], body[i+1:], true
	}
	if hadT && timePart == "" {
		return nil, fmt.Errorf("invalid duration %q: empty time part", s)
	}

	dateComps, err := scanComponents(datePart, "YMD")
	if err != nil {
		return nil, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	timeComps, err := scanComponents(timePart, "HMS")
	if err != nil {
		return nil, fmt.Errorf("invalid duration %q: %w", s, err)
	}

	_, hasY := dateComps['Y']
	_, hasMonth := dateComps['M']
	_, hasDay := dateComps['D']
	hasYM := hasY || hasMonth
	hasDT := hasDay || len(timeComps) > 0
	switch {
	case hasYM && hasDT:
		return nil, fmt.Errorf("invalid duration %q: mixes year/month and day/time", s)
	case !hasYM && !hasDT:
		return nil, fmt.Errorf("invalid duration %q: empty", s)
	}

	if hasYM {
		total := dateComps['Y']*12 + dateComps['M']
		if neg {
			total = -total
		}
		return YearsMonthsDuration{months: total}, nil
	}
	total := time.Duration(dateComps['D'])*24*time.Hour +
		time.Duration(timeComps['H'])*time.Hour +
		time.Duration(timeComps['M'])*time.Minute +
		time.Duration(timeComps['S'])*time.Second
	if neg {
		total = -total
	}
	return DaysTimeDuration{d: total}, nil
}

// scanComponents reads a sequence of "<digits><unit>" pairs, where units must
// appear in the order given and each at most once. It returns the parsed values
// keyed by unit byte.
func scanComponents(s, units string) (map[byte]int64, error) {
	res := map[byte]int64{}
	ui := 0
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i {
			return nil, fmt.Errorf("expected a number at %q", s[i:])
		}
		if j >= len(s) {
			return nil, fmt.Errorf("number %q has no unit", s[i:j])
		}
		unit := s[j]
		pos := strings.IndexByte(units[ui:], unit)
		if pos < 0 {
			return nil, fmt.Errorf("unexpected or out-of-order unit %q", string(unit))
		}
		ui += pos + 1
		n, err := strconv.ParseInt(s[i:j], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("number %q out of range", s[i:j])
		}
		res[unit] = n
		i = j + 1
	}
	return res, nil
}

// --- arithmetic helpers ---

// addMonths adds n months to t, clamping the day to the last valid day of the
// resulting month (FEEL semantics: 2024-01-31 + 1 month = 2024-02-29).
func addMonths(t time.Time, n int64) time.Time {
	total := int64(t.Year())*12 + int64(t.Month()) - 1 + n
	year := int(total / 12)
	month := time.Month(total%12) + 1
	if month < 1 {
		month += 12
		year--
	}
	day := t.Day()
	if last := lastDayOfMonth(year, month); day > last {
		day = last
	}
	return time.Date(year, month, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

func lastDayOfMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
