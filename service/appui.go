package service

import (
	_ "embed"
	"html"
	"io/fs"
	"net/http"
	"strings"

	webui "github.com/pblumer/temis/web"
)

// ogImage is the 1200×630 link-preview card (Open Graph) served at /og-image.png
// and referenced by the frontend's og:image/twitter:image tags. Embedded at build
// time so the binary ships its own preview with no external assets.
//
//go:embed og-image.png
var ogImage []byte

// appIndex is the built frontend's index.html, read once from the embedded dist.
// It carries __OG_BASE__ placeholders (see web/index.html) that handleAppIndex
// fills with the request's absolute origin so link previews resolve.
var appIndex = mustReadAppIndex()

func mustReadAppIndex() []byte {
	b, err := fs.ReadFile(webui.Assets(), "index.html")
	if err != nil {
		// dist/index.html is embedded at compile time; a failure here is a build bug.
		panic("service: embedded frontend index.html missing: " + err.Error())
	}
	return b
}

// handleAppIndex serves the SPA shell (index.html) with the Open Graph URLs made
// absolute for the current request, so shared links unfurl a preview in Teams,
// Slack, etc. All other frontend assets are served statically by the file server.
func (s *Server) handleAppIndex(w http.ResponseWriter, r *http.Request) {
	page := strings.ReplaceAll(string(appIndex), "__OG_BASE__", html.EscapeString(baseURL(r)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

// handleOGImage serves the embedded link-preview card. Always public so crawlers
// (Teams, Slack, …) can fetch it without a token.
func (s *Server) handleOGImage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(ogImage)
}

// baseURL reconstructs the absolute origin (scheme://host) of the request,
// honoring the reverse-proxy headers X-Forwarded-Proto/Host so og:* URLs are
// correct behind a proxy or TLS terminator.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := firstField(r.Header.Get("X-Forwarded-Proto")); p != "" {
		scheme = p
	}
	host := r.Host
	if h := firstField(r.Header.Get("X-Forwarded-Host")); h != "" {
		host = h
	}
	return scheme + "://" + host
}

// firstField returns the first comma-separated, trimmed value of a header that
// proxies may set as a list (e.g. "https, http").
func firstField(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}
