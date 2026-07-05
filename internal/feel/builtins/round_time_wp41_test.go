package builtins

import (
	"testing"
	"time"

	"github.com/pblumer/temis/internal/value"
)

// TestRoundScaleRange covers WP-41.19: the round functions require a scale within
// the decimal128 exponent range [-6111, 6176]; outside yields null, and a very
// large but valid scale returns the value unchanged (TCK 1141–1144).
func TestRoundScaleRange(t *testing.T) {
	run(t, []tc{
		{name: "round up", args: []value.Value{num("5.5"), num("6176")}, want: "5.5"},
		{name: "round up", args: []value.Value{num("5.5"), num("6177")}, wantNull: true},
		{name: "round up", args: []value.Value{num("5.5"), num("-6112")}, wantNull: true},
		{name: "round down", args: []value.Value{num("5.5"), num("6176")}, want: "5.5"},
		{name: "round half up", args: []value.Value{num("5.5"), num("-6111")}, want: "0"},
		{name: "decimal", args: []value.Value{num("1.5"), num("6177")}, wantNull: true},
	})
}

// TestTimeConstructorAndOffsetRendering covers WP-41.19: time() accepts a date
// (midnight UTC), and a fixed offset with a seconds component renders as
// ±HH:MM:SS (TCK 1116).
func TestTimeConstructorAndOffsetRendering(t *testing.T) {
	// time(date) → 00:00:00Z
	got := call(t, "time", mustDate("2017-08-10"))
	if got.String() != "00:00:00Z" {
		t.Errorf("time(date) = %s, want 00:00:00Z", got.String())
	}
	// an offset with seconds keeps its seconds field when rendered
	off := value.NewDaysTimeDuration(2*time.Hour + 45*time.Minute + 55*time.Second) // +02:45:55
	tv := value.NewTime(11, 59, 45, 0, &off)
	if tv.String() != "11:59:45+02:45:55" {
		t.Errorf("time offset render = %s, want 11:59:45+02:45:55", tv.String())
	}
}
