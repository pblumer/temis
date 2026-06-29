// Command temis-mcp serves the Temis DMN engine to AI agents over the Model
// Context Protocol (JSON-RPC 2.0). It is the Agent-First entry point from
// ADR-0013 (WP-50): an agent calls this server as a native tool to delegate
// rule-based decisions and get deterministic, reproducible answers.
//
// Two transports (ADR-0015):
//   - stdio (default): the agent launches this binary as a local subprocess.
//     stdout carries the protocol; all logging goes to stderr so it never
//     corrupts the message stream.
//   - HTTP (-http host:port): MCP Streamable HTTP, reachable remotely (e.g.
//     routed behind a reverse proxy). Optionally gated by a bearer token.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/internal/version"
	"github.com/pblumer/temis/mcp"
)

func main() {
	showVersion := flag.Bool("version", false, "print the temis-mcp version and exit")
	httpAddr := flag.String("http", "", "serve MCP over HTTP on this address (host:port) instead of stdio; empty = stdio")
	token := flag.String("token", os.Getenv("TEMIS_API_TOKEN"),
		"require this bearer token on the HTTP endpoint (default $TEMIS_API_TOKEN; empty = open); ignored for stdio")
	flag.Parse()

	ver := version.Resolve()
	if *showVersion {
		fmt.Printf("temis-mcp %s\n", ver)
		return
	}

	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	srv := mcp.NewServer(dmn.New(), mcp.WithVersion(ver), mcp.WithHTTPToken(*token))

	if *httpAddr != "" {
		if *token != "" {
			log.Printf("temis-mcp: HTTP endpoint requires a bearer token")
		}
		log.Printf("temis-mcp %s: serving MCP over HTTP on %s (POST /mcp)", ver, *httpAddr)
		if err := http.ListenAndServe(*httpAddr, srv.HTTPHandler()); err != nil {
			fmt.Fprintf(os.Stderr, "temis-mcp: %v\n", err)
			os.Exit(1)
		}
		return
	}

	log.Printf("temis-mcp %s: serving MCP over stdio", ver)
	if err := srv.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "temis-mcp: %v\n", err)
		os.Exit(1)
	}
}
