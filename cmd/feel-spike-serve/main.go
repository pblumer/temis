// Command feel-spike-serve is a zero-dependency static file server for the
// FEEL-WASM spike (ADR-0016, Gate 2). It exists only so the page is reachable
// over http:// (browsers refuse to fetch a .wasm from file://) and so .wasm is
// served with the correct MIME type. Build the artifacts first with
// ./web/feel-spike/build.sh (or `make feel-spike`).
package main

import (
	"flag"
	"log"
	"mime"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":8090", "listen address")
	dir := flag.String("dir", "web/feel-spike", "directory to serve")
	flag.Parse()

	// Ensure WebAssembly streaming/typing works even on minimal MIME tables.
	_ = mime.AddExtensionType(".wasm", "application/wasm")

	log.Printf("serving %s on http://localhost%s (open in a browser)", *dir, *addr)
	if err := http.ListenAndServe(*addr, http.FileServer(http.Dir(*dir))); err != nil {
		log.Fatal(err)
	}
}
