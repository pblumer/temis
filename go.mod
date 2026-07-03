module github.com/pblumer/temis

// Minimum Go 1.24: several stdlib CVEs the govulncheck gate (WP-137) flags —
// notably GO-2025-4007 (quadratic crypto/x509 name-constraint check, reachable
// via ListenAndServeTLS) — are fixed only in Go 1.24.9+, never backported to the
// EOL 1.23 line. Building on 1.24 is required to actually remediate, not just to
// pass the scan. The CI security lane scans with the latest stable Go; the
// release image (Dockerfile) builds on 1.24.
go 1.24.0

require (
	connectrpc.com/connect v1.18.1
	github.com/cockroachdb/apd/v3 v3.2.3
	golang.org/x/net v0.41.0
	google.golang.org/protobuf v1.36.6
)

require golang.org/x/text v0.26.0 // indirect
