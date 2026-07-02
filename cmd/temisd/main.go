// Command temisd is the Temis DMN service binary. It serves the HTTP API
// (docs/40-api-contract.md §2) over the public dmn engine; the gRPC interface
// follows in WP-33.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/internal/version"
	"github.com/pblumer/temis/mcp"
	"github.com/pblumer/temis/service"
)

// defaultClioURL is the hosted clio the decision audit log points at unless
// overridden. The sink still stays off until a TEMIS_CLIO_TOKEN is provided, so a
// default start never sends decision data anywhere; setting the token is the single
// opt-in step (or point -clio-url at your own clio).
const defaultClioURL = "https://clio.blumer.cloud"

// Zero-config by design: running the binary with no flags starts a fully featured
// server (modeler, examples, MCP, model listing, and the modeling assistant in BYOK
// mode). Every default below is sourced from a TEMIS_* environment variable, so an
// operator opts out of any feature via the environment alone — no flags required
// (handy for containers). An explicit flag always overrides the environment.
func main() {
	// Offline key CLI (WP-104): `temisd keys …` manages the persistent keystore
	// directly (server stopped), for lockout recovery. It is a distinct entrypoint
	// from the server, so dispatch before the server's flag set is parsed.
	if len(os.Args) > 1 && os.Args[1] == "keys" {
		os.Exit(runKeysCommand(os.Args[2:]))
	}

	showVersion := flag.Bool("version", false, "print the temisd version and exit")
	addr := flag.String("addr", envOr("TEMIS_ADDR", ":8080"),
		"address to listen on (host:port) (default $TEMIS_ADDR, else :8080)")
	token := flag.String("token", os.Getenv("TEMIS_API_TOKEN"),
		"DEPRECATED legacy admin token on /v1 endpoints; use -keys-file for scoped keys (default $TEMIS_API_TOKEN; empty = none)")
	keysFile := flag.String("keys-file", os.Getenv("TEMIS_KEYS_FILE"),
		"JSON file of scoped kid.secret API keys guarding /v1, /mcp and gRPC (default $TEMIS_KEYS_FILE; empty = none)")
	keysDir := flag.String("keys-dir", os.Getenv("TEMIS_KEYS_DIR"),
		"directory for the persistent managed keystore + lifecycle API (POST /v1/keys …); keys survive a restart; empty = key management off (default $TEMIS_KEYS_DIR)")
	listModels := flag.Bool("list-models", envBool("TEMIS_LIST_MODELS", true),
		"expose GET /v1/models, which lists every cached model; set false to keep decisions private (env TEMIS_LIST_MODELS)")
	cacheSize := flag.Int("cache-size", envInt("TEMIS_CACHE_SIZE", 0),
		"max compiled models kept in memory (LRU eviction); 0 uses the default, negative means unbounded (env TEMIS_CACHE_SIZE)")
	maxCallDepth := flag.Int("max-call-depth", envInt("TEMIS_MAX_CALL_DEPTH", 0), "limit on nested function/BKM recursion (0 = default) (env TEMIS_MAX_CALL_DEPTH)")
	maxIterations := flag.Int("max-iterations", envInt("TEMIS_MAX_ITERATIONS", 0), "limit on total comprehension iterations per evaluation (0 = default) (env TEMIS_MAX_ITERATIONS)")
	maxListSize := flag.Int("max-list-size", envInt("TEMIS_MAX_LIST_SIZE", 0), "limit on the size of any single produced list (0 = default) (env TEMIS_MAX_LIST_SIZE)")
	examples := flag.Bool("examples", envBool("TEMIS_EXAMPLES", true),
		"preload the bundled example DMN models so they appear in the modeler on start (env TEMIS_EXAMPLES)")
	modelsDir := flag.String("models-dir", os.Getenv("TEMIS_MODELS_DIR"),
		"persist uploaded/edited models to this directory and reload them on start, so they survive a restart; empty = in-memory only (default $TEMIS_MODELS_DIR)")
	flowsDir := flag.String("flows-dir", os.Getenv("TEMIS_FLOWS_DIR"),
		"load decision-flow descriptors (*.flow.json) from this directory into the catalog on start (read-only source of truth); empty = catalog starts empty (default $TEMIS_FLOWS_DIR)")
	serveMCP := flag.Bool("mcp", envBool("TEMIS_MCP", true),
		"co-locate the MCP endpoint at POST /mcp, sharing this server's model cache (and examples) (env TEMIS_MCP)")
	assist := flag.Bool("assist", envBool("TEMIS_ASSIST", true),
		"enable the modeling assistant at POST /v1/chat; on by default (BYOK unless a server key is set), set false to disable (env TEMIS_ASSIST)")
	llmProvider := flag.String("llm-provider", os.Getenv("TEMIS_LLM_PROVIDER"),
		"LLM provider for the modeling assistant: \"anthropic\" or \"openai\" (default $TEMIS_LLM_PROVIDER, else anthropic)")
	llmToken := flag.String("llm-token", os.Getenv("TEMIS_LLM_TOKEN"),
		"server-side API key for the LLM provider (default $TEMIS_LLM_TOKEN; empty = BYOK only)")
	llmModel := flag.String("llm-model", os.Getenv("TEMIS_LLM_MODEL"),
		"override the provider's default model id (default $TEMIS_LLM_MODEL)")
	llmBaseURL := flag.String("llm-base-url", os.Getenv("TEMIS_LLM_BASE_URL"),
		"override the provider's API base URL, e.g. a proxy (default $TEMIS_LLM_BASE_URL)")
	llmAllowBYOK := flag.Bool("llm-allow-byok", envBool("TEMIS_LLM_ALLOW_BYOK", true),
		"let a caller supply its own provider key via the X-LLM-Token header (used only for that request) (env TEMIS_LLM_ALLOW_BYOK)")
	clioURL := flag.String("clio-url", envOr("TEMIS_CLIO_URL", defaultClioURL),
		"clio instance that receives each evaluation as a tamper-evident event (default $TEMIS_CLIO_URL, else the hosted "+defaultClioURL+")")
	clioToken := flag.String("clio-token", os.Getenv("TEMIS_CLIO_TOKEN"),
		"clio API key (kid.secret); the audit sink stays OFF until this is set, so no decision data leaves the process by default (default $TEMIS_CLIO_TOKEN)")
	clioSource := flag.String("clio-source", os.Getenv("TEMIS_CLIO_SOURCE"),
		"CloudEvents source stamped on audit events (default $TEMIS_CLIO_SOURCE, else \"temisd\")")
	clioSubjectPrefix := flag.String("clio-subject-prefix", envOr("TEMIS_CLIO_SUBJECT_PREFIX", "/decisions"),
		"clio subject prefix the decision is filed under (env TEMIS_CLIO_SUBJECT_PREFIX)")
	clioSubjectKey := flag.String("clio-subject-key", os.Getenv("TEMIS_CLIO_SUBJECT_KEY"),
		"input field whose value becomes the subject's entity segment (empty = decision name) (env TEMIS_CLIO_SUBJECT_KEY)")
	clioStrict := flag.Bool("clio-strict", envBool("TEMIS_CLIO_STRICT", false),
		"fail-closed: abort the evaluation (502) if the audit write fails (default best-effort: log and continue) (env TEMIS_CLIO_STRICT)")
	clioActiveProbe := flag.Bool("clio-active-probe", envBool("TEMIS_CLIO_ACTIVE_PROBE", false),
		"GET /v1/status actively pings clio's health endpoint for reachability instead of using the passive last-write outcome (env TEMIS_CLIO_ACTIVE_PROBE)")
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
	// Scoped API keys (ADR-0028). The bootstrap admin key is env-only so a secret
	// never lands in a process listing or shell history via a flag.
	bootstrapAdminKey := os.Getenv("TEMIS_BOOTSTRAP_ADMIN_KEY")
	opts := []service.Option{
		service.WithToken(*token),
		service.WithKeysFile(*keysFile),
		service.WithBootstrapAdminKey(bootstrapAdminKey),
		service.WithKeyStore(*keysDir),
		service.WithModelListing(*listModels),
		service.WithVersion(ver),
	}
	if *clioActiveProbe {
		opts = append(opts, service.WithClioActiveProbe(true))
	}
	if *cacheSize != 0 {
		opts = append(opts, service.WithCacheSize(*cacheSize))
	}
	if *examples {
		opts = append(opts, service.WithExamples())
	}
	if *modelsDir != "" {
		opts = append(opts, service.WithModelStore(*modelsDir))
	}
	if *flowsDir != "" {
		opts = append(opts, service.WithFlowStore(*flowsDir))
	}
	// Modeling assistant (ADR-0024): on by default so the binary is fully featured
	// out of the box. With no server-side key it runs BYOK-only — the endpoint is
	// live and answers once a caller supplies its own key via X-LLM-Token. It is only
	// left unmounted when explicitly disabled, or when there is no way to obtain a key
	// (no server token and BYOK off). Provider defaults to anthropic.
	assistOn := *assist && (*llmToken != "" || *llmAllowBYOK)
	if assistOn {
		provider := *llmProvider
		if provider == "" {
			provider = "anthropic"
		}
		opts = append(opts, service.WithAssist(service.AssistConfig{
			Provider:  provider,
			Token:     *llmToken,
			Model:     *llmModel,
			BaseURL:   *llmBaseURL,
			AllowBYOK: *llmAllowBYOK,
		}))
	}
	// Decision audit log (ADR-0023): the URL defaults to the hosted clio, but the
	// sink is gated on the token so a default start never sends decision data
	// anywhere. Providing a token is the single opt-in step.
	clioOn := *clioToken != "" && *clioURL != ""
	var qq *service.QualityQueue
	if clioOn {
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
		// Quality events for productive Import runs drain through a decoupled,
		// guaranteed-delivery queue (backpressure, retry) so a big batch's writes
		// never block the response and survive a transient clio hiccup.
		qq = service.NewQualityQueue(sink, service.QualityQueueConfig{})
		opts = append(opts, service.WithQualityQueue(qq))
	}
	srv := service.NewServer(engine, opts...)
	if *serveMCP {
		// One address space: the MCP endpoint shares the service's model cache, so
		// the preloaded examples (and any API-loaded model) are visible over MCP,
		// and models loaded over MCP appear in the modeler. The same optional token
		// guards /mcp as the /v1 endpoints.
		mcpSrv := mcp.NewServer(engine,
			mcp.WithVersion(ver),
			mcp.WithAuth(srv.MCPAuth()),
			mcp.WithStore(srv.ModelStore()),
		)
		srv.AttachMCP(mcpSrv)
	}
	switch {
	case *keysFile != "" || bootstrapAdminKey != "":
		log.Printf("temisd: /v1, /mcp and gRPC require a scoped API key (kid.secret)")
		if *token != "" {
			log.Printf("temisd: DEPRECATED -token / TEMIS_API_TOKEN accepted as a legacy admin key")
		}
	case *token != "":
		log.Printf("temisd: /v1, /mcp and gRPC require the DEPRECATED legacy admin token — migrate to -keys-file (ADR-0028)")
	}
	if !*listModels {
		log.Printf("temisd: GET /v1/models listing disabled")
	}
	if *modelsDir != "" {
		log.Printf("temisd: persisting models to %s (survives restart)", *modelsDir)
	}
	if *serveMCP {
		log.Printf("temisd: MCP endpoint at POST /mcp (shared model cache)")
	}
	if assistOn {
		provider := *llmProvider
		if provider == "" {
			provider = "anthropic"
		}
		keying := "server key"
		if *llmToken == "" {
			keying = "BYOK only (send X-LLM-Token)"
		}
		log.Printf("temisd: modeling assistant at POST /v1/chat (provider %q, %s, BYOK=%v)", provider, keying, *llmAllowBYOK)
	}
	if clioOn {
		mode := "best-effort"
		if *clioStrict {
			mode = "fail-closed"
		}
		log.Printf("temisd: clio audit sink → %s (%s)", *clioURL, mode)
		log.Printf("temisd: clio quality events for productive Import runs → %s%s (guaranteed queue)", *clioURL, "/quality")
	} else {
		// Advertise the sister project without sending anything: the sink is one
		// token away. https://github.com/pblumer/clio
		log.Printf("temisd: tamper-evident decision log available — set TEMIS_CLIO_TOKEN to record to %s (clio, or -clio-url your own)", *clioURL)
	}
	log.Printf("temisd %s listening on %s — DMN modeler at http://%s/ · Swagger UI at http://%s/docs · gRPC (dmn.v1.DmnEngine) on the same port",
		ver, *addr, *addr, *addr)

	// Graceful shutdown so the quality queue drains before exit (guaranteed
	// delivery). On SIGINT/SIGTERM: stop accepting requests, then drain the queue
	// under a deadline so an unreachable clio can't hang the shutdown.
	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler()}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		if qq != nil {
			qq.Close(ctx)
		}
	}()
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "temisd: %v\n", err)
		os.Exit(1)
	}
}

// envOr returns the value of environment variable key, or def when it is unset.
func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// envBool reads a boolean environment variable (1/t/true/0/f/false, case-insensitive
// per strconv.ParseBool), falling back to def when the variable is unset or unparseable.
func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return b
}

// envInt reads an integer environment variable, falling back to def when the
// variable is unset or unparseable.
func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}
