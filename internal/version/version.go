// Package version exposes the build version of the Temis binaries.
package version

import "runtime/debug"

// Version is the Temis version. The release pipeline (WP-46) stamps the real
// version at build time via the linker:
//
//	go build -ldflags "-X github.com/pblumer/temis/internal/version.Version=v1.2.3" ./cmd/temisd
//
// It is a var (not a const) precisely so -ldflags -X can override it. When left
// at the development placeholder — e.g. after `go install …@v1.2.3` without the
// linker flag — [Resolve] falls back to the module version recorded in the
// build info.
var Version = "0.0.0-dev"

// Resolve returns the build version: the linker-stamped [Version] when set,
// otherwise the module version from the build info (so `go install …@vX` still
// reports a meaningful version), and finally the development placeholder.
func Resolve() string {
	if Version != "" && Version != "0.0.0-dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return Version
}
