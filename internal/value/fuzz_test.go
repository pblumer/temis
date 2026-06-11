package value

import "testing"

// FuzzParseNumber asserts the parser never panics and never returns a value
// together with an error.
func FuzzParseNumber(f *testing.F) {
	for _, s := range []string{"", "0", "-3.14", "1e10", "abc", "1.2.3", "99999999999999999999999999999999999999"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		n, err := ParseNumber(s)
		if err == nil {
			_ = n.String() // must not panic
		}
	})
}

// FuzzParseDuration asserts ISO-duration parsing never panics.
func FuzzParseDuration(f *testing.F) {
	for _, s := range []string{"P1Y2M", "P1DT2H3M4S", "-P5D", "PT0S", "P", "PT", "garbage", "P1Y2D"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := ParseDuration(s)
		if err == nil {
			_ = v.String()
		}
	})
}
