package feel

import "testing"

// TestTemporalEndToEnd covers WP-22 through the full pipeline: temporal
// constructor builtins, @-literals, component path access, and multi-word names
// whose fragments include the "and" keyword (resolved via the registry oracle).
func TestTemporalEndToEnd(t *testing.T) {
	cases := map[string]string{
		// constructors
		`date(2024, 2, 29)`:                                  "2024-02-29",
		`date("2024-02-29")`:                                 "2024-02-29",
		`time(13, 30, 0)`:                                    "13:30:00",
		`date and time("2024-02-29T13:30:00")`:               "2024-02-29T13:30:00",
		`date and time(date("2024-02-29"), time(13, 30, 0))`: "2024-02-29T13:30:00",

		// @-literals
		`@"2024-02-29"`:          "2024-02-29",
		`@"P1Y6M"`:               "P1Y6M",
		`@"2024-02-29T08:00:00"`: "2024-02-29T08:00:00",

		// component access
		`date("2024-02-29").year`:     "2024",
		`date("2024-02-29").month`:    "2",
		`date("2024-02-29").weekday`:  "4",
		`@"2024-02-29T13:30:45".hour`: "13",
		`duration("P1Y6M").years`:     "1",
		`duration("P1Y6M").months`:    "6",
		`duration("P2DT3H").days`:     "2",
		`duration("P2DT3H").hours`:    "3",

		// calendar accessors
		`day of week(date("2024-02-29"))`:   "Thursday",
		`month of year(date("2024-02-29"))`: "February",

		// arithmetic + difference
		`date("2024-01-31") + duration("P1M")`:                         "2024-02-29",
		`years and months duration(date("2020-01-01"), @"2021-06-15")`: "P1Y5M",

		// multi-word member names (FEEL names may contain spaces) on temporals
		`time("10:30:00+05:00").time offset`:                    "P0DT5H0M0S",
		`date and time("2018-12-10T10:30:00@Etc/UTC").timezone`: "Etc/UTC",

		// range endpoint properties, including inclusivity (multi-word names)
		`[1..10].start`:          "1",
		`[1..10].end`:            "10",
		`[1..10].start included`: "true",
		`(1..10].start included`: "false",
		`[1..10).end included`:   "false",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}
