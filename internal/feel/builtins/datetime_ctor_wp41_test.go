package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestDateAndTimeConstructorForms covers the constructor overloads added for the
// TCK 1117 suite: a date-only string promotes to start-of-day, and the two-arg
// form accepts a date-and-time (its date part) as well as a plain date.
func TestDateAndTimeConstructorForms(t *testing.T) {
	dt := call(t, "date and time", str("2017-08-10T10:20:00")) // a DateTime value
	tm := value.NewTime(23, 59, 1, 0, nil)

	cases := []struct {
		name string
		args []value.Value
		want string
	}{
		{"date-only string ⇒ midnight", []value.Value{str("2012-12-24")}, "2012-12-24T00:00:00"},
		{"dateAndTime + time ⇒ recombined", []value.Value{dt, tm}, "2017-08-10T23:59:01"},
		{"date + time", []value.Value{mustDate("2017-01-01"), tm}, "2017-01-01T23:59:01"},
		{"large year string", []value.Value{str("99999-12-31T11:22:33")}, "99999-12-31T11:22:33"},
	}
	for _, c := range cases {
		got := call(t, "date and time", c.args...)
		if got.String() != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}
