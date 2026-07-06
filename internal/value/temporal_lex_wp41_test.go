package value

import "testing"

// TestStrictTemporalLexis pins the malformed date/time strings that FEEL requires
// to be rejected (the constructors turn the error into null). See the 1115/1116/
// 1117 TCK suites.
func TestStrictTemporalLexis(t *testing.T) {
	badDates := []string{
		"9999999999-12-25", // year > 9 digits (magnitude > 999999999)
		"998-12-31",        // year < 4 digits
		"01211-12-31",      // 5-digit year with a leading zero
		"+2012-12-02",      // leading '+' sign
		"2012-13-01",       // month out of range
		"2012-12-32",       // day out of range
	}
	for _, s := range badDates {
		if _, err := ParseDate(s); err == nil {
			t.Errorf("ParseDate(%q) accepted, want error", s)
		}
	}

	badTimes := []string{
		"7:00:00",        // single-digit hour
		"13:20:00+19:00", // offset beyond ±18:00
		"13:20:00-19:00",
		"25:00:00", // hour out of range
	}
	for _, s := range badTimes {
		if _, err := ParseTime(s); err == nil {
			t.Errorf("ParseTime(%q) accepted, want error", s)
		}
	}

	badDateTimes := []string{
		"9999999999-12-27T11:22:33",
		"998-12-31T11:22:33",
		"01211-12-31T11:22:33",
		"+99999-12-01T11:22:33",
		"2017-12-31T7:00:00",        // single-digit hour
		"2017-12-31T13:20:00+19:00", // offset out of range
	}
	for _, s := range badDateTimes {
		if _, err := ParseDateTime(s); err == nil {
			t.Errorf("ParseDateTime(%q) accepted, want error", s)
		}
	}

	// Well-formed values at the edges must still parse.
	good := []string{"0998-12-31", "999999999-12-31"}
	for _, s := range good {
		if _, err := ParseDate(s); err != nil {
			t.Errorf("ParseDate(%q) rejected: %v", s, err)
		}
	}
	if _, err := ParseTime("23:59:59.999+14:00"); err != nil {
		t.Errorf("ParseTime(+14:00) rejected: %v", err)
	}
}
