// Command temisd is the Temis DMN service binary. It will host the HTTP and
// gRPC interfaces (WP-32/WP-33); for now it only reports its version, which
// keeps the scaffold buildable and runnable.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pblumer/temis/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the temisd version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("temisd %s\n", version.Version)
		return
	}

	fmt.Fprintf(os.Stderr, "temisd %s: service not yet implemented (see docs/20-roadmap.md, WP-32)\n", version.Version)
	fmt.Fprintln(os.Stderr, "usage: temisd --version")
}
