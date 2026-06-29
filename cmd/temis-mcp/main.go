// Command temis-mcp serves the Temis DMN engine to AI agents over the Model
// Context Protocol (JSON-RPC 2.0 over stdio). It is the Agent-First entry point
// from ADR-0013 (WP-50): an agent launches this binary as a subprocess and calls
// temis as a native tool to delegate rule-based decisions and get deterministic,
// reproducible answers.
//
// stdout carries the protocol; all logging goes to stderr so it never corrupts
// the message stream.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/internal/version"
	"github.com/pblumer/temis/mcp"
)

func main() {
	showVersion := flag.Bool("version", false, "print the temis-mcp version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("temis-mcp %s\n", version.Version)
		return
	}

	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	srv := mcp.NewServer(dmn.New(), mcp.WithVersion(version.Version))
	log.Printf("temis-mcp %s: serving MCP over stdio", version.Version)
	if err := srv.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "temis-mcp: %v\n", err)
		os.Exit(1)
	}
}
