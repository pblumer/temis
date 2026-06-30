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
	"github.com/pblumer/temis/mcp"
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
	examples := flag.Bool("examples", true,
		"preload the bundled example DMN models so they appear in the modeler on start")
	serveMCP := flag.Bool("mcp", true,
		"co-locate the MCP endpoint at POST /mcp, sharing this server's model cache (and examples)")
	clioURL := flag.String("clio-url", os.Getenv("TEMIS_CLIO_URL"),
		"record each evaluation as a tamper-evident event in this clio instance (default $TEMIS_CLIO_URL; empty = off)")
	clioToken := flag.String("clio-token", os.Getenv("TEMIS_CLIO_TOKEN"),
		"clio API key (kid.secret) for the audit sink (default $TEMIS_CLIO_TOKEN)")
	clioSource := flag.String("clio-source", os.Getenv("TEMIS_CLIO_SOURCE"),
		"CloudEvents source stamped on audit events (default $TEMIS_CLIO_SOURCE, else \"temisd\")")
	clioSubjectPrefix := flag.String("clio-subject-prefix", "/decisions",
		"clio subject prefix the decision is filed under")
	clioSubjectKey := flag.String("clio-subject-key", "",
		"input field whose value becomes the subject's entity segment (empty = decision name)")
	clioStrict := flag.Bool("clio-strict", false,
		"fail-closed: abort the evaluation (502) if the audit write fails (default best-effort: log and continue)")
	flag.Parse()

	ver := version.Resolve()
	if *showVersion {
		fmt.Printf("temisd %s\n", ver)
		return
	}

	engine := dmn.New(dmn.WithLimits(dmn.Limits{
		MaxCallDepth:  *maxCallDepth,
		MaxIterations: *maxIterations,
		MaxListSize:   *maxListSize,
	}))
	opts := []service.Option{
		service.WithToken(*token),
		service.WithModelListing(*listModels),
	}
	if *cacheSize != 0 {
		opts = append(opts, service.WithCacheSize(*cacheSize))
	}
	if *examples {
		opts = append(opts, service.WithExamples())
	}
	if *clioURL != "" {
		sink, err := service.NewClioSink(service.ClioConfig{
			URL:           *clioURL,
			Token:         *clioToken,
			Source:        *clioSource,
			SubjectPrefix: *clioSubjectPrefix,
			SubjectKey:    *clioSubjectKey,
			Engine:        "temisd " + ver,
			Strict:        *clioStrict,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "temisd: %v\n", err)
			os.Exit(1)
		}
		opts = append(opts, service.WithClioSink(sink))
	}
	srv := service.NewServer(engine, opts...)
	if *serveMCP {
		// One address space: the MCP endpoint shares the service's model cache, so
		// the preloaded examples (and any API-loaded model) are visible over MCP,
		// and models loaded over MCP appear in the modeler. The same optional token
		// guards /mcp as the /v1 endpoints.
		mcpSrv := mcp.NewServer(engine,
			mcp.WithVersion(ver),
			mcp.WithHTTPToken(*token),
			mcp.WithStore(srv.ModelStore()),
		)
		srv.AttachMCP(mcpSrv)
	}
	if *token != "" {
		log.Printf("temisd: /v1 endpoints require a bearer token")
	}
	if !*listModels {
		log.Printf("temisd: GET /v1/models listing disabled")
	}
	if *serveMCP {
		log.Printf("temisd: MCP endpoint at POST /mcp (shared model cache)")
	}
	if *clioURL != "" {
		mode := "best-effort"
		if *clioStrict {
			mode = "fail-closed"
		}
		log.Printf("temisd: clio audit sink → %s (%s)", *clioURL, mode)
	}
	log.Printf("temisd %s listening on %s — DMN modeler at http://%s/ · Swagger UI at http://%s/docs · gRPC (dmn.v1.DmnEngine) on the same port",
		ver, *addr, *addr, *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "temisd: %v\n", err)
		os.Exit(1)
	}
}
