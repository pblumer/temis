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
	listModels := flag.Bool("list-models", true,
		"expose GET /v1/models, which lists every cached model; set false to keep decisions private")
	cacheSize := flag.Int("cache-size", 0,
		"max compiled models kept in memory (LRU eviction); 0 uses the default, negative means unbounded")
	maxCallDepth := flag.Int("max-call-depth", 0, "limit on nested function/BKM recursion (0 = default)")
	maxIterations := flag.Int("max-iterations", 0, "limit on total comprehension iterations per evaluation (0 = default)")
	maxListSize := flag.Int("max-list-size", 0, "limit on the size of any single produced list (0 = default)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("temisd %s\n", version.Version)
		return
	}

	engine := dmn.New(dmn.WithLimits(dmn.Limits{
		MaxCallDepth:  *maxCallDepth,
		MaxIterations: *maxIterations,
		MaxListSize:   *maxListSize,
	}))
	srvOpts := []service.Option{
		service.WithToken(*token),
		service.WithModelListing(*listModels),
	}
	if *cacheSize != 0 {
		srvOpts = append(srvOpts, service.WithCacheSize(*cacheSize))
	}
	srv := service.NewServer(engine, srvOpts...)
	if *token != "" {
		log.Printf("temisd: /v1 endpoints require a bearer token")
	}
	if !*listModels {
		log.Printf("temisd: GET /v1/models listing disabled")
	}
	log.Printf("temisd %s listening on %s — Playground at http://%s/ui · Swagger UI at http://%s/docs",
		version.Version, *addr, *addr, *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "temisd: %v\n", err)
		os.Exit(1)
	}
}
