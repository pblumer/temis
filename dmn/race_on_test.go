//go:build race

package dmn_test

// raceEnabled is true when the test binary is built with the race detector, which
// inflates timings and allocations; the performance budget skips itself then and
// is enforced by the race-free `make budget` target instead.
const raceEnabled = true
