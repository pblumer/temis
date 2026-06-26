// Command temisd is the Temis DMN service binary. It serves the HTTP API
// (docs/40-api-contract.md §2) over the public dmn engine; the gRPC interface
// follows in WP-33.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/internal/version"
	"github.com/pblumer/temis/service"
)

func main() {
	showVersion := flag.Bool("version", false, "print the temisd version and exit")
	addr := flag.String("addr", ":8080", "address to listen on (host:port)")
	token := flag.String("token", os.Getenv("TEMIS_API_TOKEN"),
		"require this bearer token on /v1 endpoints (default $TEMIS_API_TOKEN; empty = open)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("temisd %s\n", version.Version)
		return
	}

	srv := service.NewServer(dmn.New(), service.WithToken(*token))
	if *token != "" {
		log.Printf("temisd: /v1 endpoints require a bearer token")
	}
	log.Printf("temisd %s listening on %s — Playground at http://%s/ui · Swagger UI at http://%s/docs",
		version.Version, *addr, *addr, *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "temisd: %v\n", err)
		os.Exit(1)
	}
}
