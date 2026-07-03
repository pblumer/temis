module github.com/pblumer/temis

go 1.23.0

// Build/scan with the latest 1.23 patch release: the language minimum stays
// 1.23.0 (no impact on library consumers), but CI — including the govulncheck
// security scan — uses a toolchain with the stdlib security fixes, rather than
// the vulnerable 1.23.0 that `go-version-file: go.mod` would otherwise pin.
toolchain go1.23.12

require (
	connectrpc.com/connect v1.18.1
	github.com/cockroachdb/apd/v3 v3.2.3
	golang.org/x/net v0.41.0
	google.golang.org/protobuf v1.36.6
)

require golang.org/x/text v0.26.0 // indirect
