// Package service hosts the HTTP and gRPC handlers that expose the engine as a
// network service. It accesses the engine only through the public dmn package,
// never through internal/.
//
// This package is currently a scaffold (WP-01); the HTTP wrapper starts at WP-32.
package service
