package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// This file makes temis a self-contained OAuth 2.1 authorization server, so a
// remote MCP client (e.g. the claude.ai web connector) can obtain a bearer for
// the co-located /mcp endpoint through the standard Authorization-Code + PKCE
// flow (ADR-0038). temis is at once the authorization server AND the resource
// server: the access token it issues is an ordinary short-lived managed key
// (ADR-0028) minted through the existing keystore, so /mcp, /v1 and gRPC accept
// it unchanged through the one Authenticator seam. No external IdP is required;
// ADR-0036's Keycloak/JWT variant remains a separate future target.
//
// Human identity at /authorize comes from a real server-side cookie session
// (session.go): the operator proves who they are once with a kid.secret, and the
// token minted for the client is bounded by that key's scopes.

const (
	// defaultAccessTTL bounds the life of an issued access token. Tokens are
	// short so a leaked one ages out quickly; the client refreshes silently.
	defaultAccessTTL = time.Hour
	// authCodeTTL is the brief window in which a freshly issued authorization code
	// may be exchanged for a token. Codes are single-use regardless.
	authCodeTTL = 60 * time.Second
)

// defaultOAuthScopes is the least-privilege set issued to a connector token: it
// covers the human⇄agent modeling roundtrip (evaluate, read/write models, flows,
// git) but never admin, the paid assistant or the audit log. Overridable with
// WithOAuthScopes / -oauth-scopes.
var defaultOAuthScopes = []Scope{ScopeEvaluate, ScopeModelsRead, ScopeModelsWrite, ScopeFlow, ScopeGit}

// defaultRedirectHosts are the redirect-target hosts allowed without explicit
// registration: the claude.ai web connector's callback and loopback for local
// MCP clients. Extra hosts come from WithOAuthRedirectAllow / -oauth-redirect-allow.
var defaultRedirectHosts = []string{"claude.ai", "localhost", "127.0.0.1"}

// oauthConfig is the resolved authorization-server configuration.
type oauthConfig struct {
	issuer        string        // canonical external base URL, no trailing slash
	grantScopes   []Scope       // the scopes a token may be issued with
	redirectHosts []string      // allowed redirect_uri hosts
	accessTTL     time.Duration // issued-token lifetime
	secure        bool          // server terminates TLS → mark cookies Secure
}

// authCode is a pending authorization code awaiting exchange at /token. It is
// bound to the client, redirect_uri, PKCE challenge and resource it was issued
// for; the token request must match all of them.
type authCode struct {
	clientID    string
	redirectURI string
	challenge   string // PKCE S256 code_challenge
	resource    string
	scopes      []Scope
	subject     string // kid that authorized it (audit)
	expires     time.Time
	used        bool
}

// refreshGrant lets a client mint a fresh access token without re-consent.
type refreshGrant struct {
	clientID string
	scopes   []Scope
	subject  string
	resource string
}

// oauthClient is a dynamically registered client (RFC 7591). temis registers
// public clients only (PKCE, no client secret).
type oauthClient struct {
	id           string
	redirectURIs []string
}

// oauthServer holds the authorization-server state. Codes, refresh tokens and
// registered clients live in memory, mutex-guarded like the keystore; access
// tokens live in the keystore itself.
type oauthServer struct {
	cfg      oauthConfig
	ks       *keystore
	sessions *sessionStore
	now      func() time.Time

	mu      sync.Mutex
	codes   map[string]*authCode
	refresh map[string]*refreshGrant
	clients map[string]*oauthClient
}

func newOAuthServer(cfg oauthConfig, ks *keystore, sessions *sessionStore) *oauthServer {
	if cfg.accessTTL <= 0 {
		cfg.accessTTL = defaultAccessTTL
	}
	if len(cfg.grantScopes) == 0 {
		cfg.grantScopes = defaultOAuthScopes
	}
	cfg.redirectHosts = append(append([]string{}, defaultRedirectHosts...), cfg.redirectHosts...)
	cfg.issuer = strings.TrimRight(cfg.issuer, "/")
	return &oauthServer{
		cfg:      cfg,
		ks:       ks,
		sessions: sessions,
		now:      time.Now,
		codes:    map[string]*authCode{},
		refresh:  map[string]*refreshGrant{},
		clients:  map[string]*oauthClient{},
	}
}

// --- discovery metadata ---------------------------------------------------

// handleAuthzMetadata serves RFC 8414 authorization-server metadata so a client
// can discover the endpoints instead of guessing them.
func (o *oauthServer) handleAuthzMetadata(w http.ResponseWriter, _ *http.Request) {
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"issuer":                                o.cfg.issuer,
		"authorization_endpoint":                o.cfg.issuer + "/authorize",
		"token_endpoint":                        o.cfg.issuer + "/token",
		"registration_endpoint":                 o.cfg.issuer + "/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      scopeStrings(o.cfg.grantScopes),
	})
}

// handleProtectedResourceMetadata serves RFC 9728 metadata pointing the client
// at this same server as its authorization server.
func (o *oauthServer) handleProtectedResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"resource":                 o.cfg.issuer,
		"authorization_servers":    []string{o.cfg.issuer},
		"scopes_supported":         scopeStrings(o.cfg.grantScopes),
		"bearer_methods_supported": []string{"header"},
	})
}

// resourceMetadataURL is the absolute URL of the protected-resource metadata,
// advertised in WWW-Authenticate on 401 (RFC 9728 §5.1).
func (o *oauthServer) resourceMetadataURL() string {
	return o.cfg.issuer + "/.well-known/oauth-protected-resource"
}

// --- dynamic client registration (RFC 7591) ------------------------------

func (o *oauthServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		oauthJSONError(w, http.StatusBadRequest, "invalid_client_metadata", "cannot parse registration body")
		return
	}
	if len(req.RedirectURIs) == 0 {
		oauthJSONError(w, http.StatusBadRequest, "invalid_redirect_uri", "at least one redirect_uri is required")
		return
	}
	for _, u := range req.RedirectURIs {
		if !o.redirectHostAllowed(u) {
			oauthJSONError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri host is not allowed: "+u)
			return
		}
	}
	id, err := randToken(16)
	if err != nil {
		oauthJSONError(w, http.StatusInternalServerError, "server_error", "could not allocate client id")
		return
	}
	clientID := "client_" + id
	o.mu.Lock()
	o.clients[clientID] = &oauthClient{id: clientID, redirectURIs: req.RedirectURIs}
	o.mu.Unlock()
	writeOAuthJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"redirect_uris":              req.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	})
}

// --- authorization endpoint ----------------------------------------------

// handleAuthorize is the GET /authorize endpoint. It validates the request,
// then either shows a login page (no session), a consent page (session), so the
// human can approve minting a token for the client.
func (o *oauthServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirectURI := q.Get("redirect_uri")
	clientID := q.Get("client_id")

	// A bad client/redirect must NOT redirect (that is the very channel an attacker
	// would abuse) — render a plain error page instead.
	if redirectURI == "" || !o.redirectAllowed(clientID, redirectURI) {
		renderError(w, http.StatusBadRequest, "Ungültige oder nicht erlaubte redirect_uri.")
		return
	}
	// From here, protocol errors are reported back to the client via the redirect.
	if q.Get("response_type") != "code" {
		redirectError(w, redirectURI, q.Get("state"), "unsupported_response_type", "only response_type=code is supported")
		return
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		redirectError(w, redirectURI, q.Get("state"), "invalid_request", "PKCE with code_challenge_method=S256 is required")
		return
	}

	sess, ok := o.sessions.sessionFromRequest(r)
	if !ok {
		renderLogin(w, "/authorize?"+r.URL.RawQuery, "")
		return
	}
	scopes := o.effectiveScopes(sess, q.Get("scope"))
	if len(scopes) == 0 {
		redirectError(w, redirectURI, q.Get("state"), "invalid_scope", "the signed-in key cannot grant any issuable scope")
		return
	}
	renderConsent(w, consentData{
		CSRF:        sess.csrf,
		ClientID:    clientID,
		RedirectURI: redirectURI,
		State:       q.Get("state"),
		Challenge:   q.Get("code_challenge"),
		Resource:    q.Get("resource"),
		Scope:       q.Get("scope"),
		Subject:     sess.subject,
		Scopes:      scopeStrings(scopes),
	})
}

// handleApprove is the POST /authorize consent submission. It re-validates
// everything (never trusting the posted hidden fields), checks the session and
// its CSRF token, then mints an authorization code and redirects to the client.
func (o *oauthServer) handleApprove(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, "Ungültiges Formular.")
		return
	}
	redirectURI := r.PostForm.Get("redirect_uri")
	clientID := r.PostForm.Get("client_id")
	state := r.PostForm.Get("state")
	if redirectURI == "" || !o.redirectAllowed(clientID, redirectURI) {
		renderError(w, http.StatusBadRequest, "Ungültige oder nicht erlaubte redirect_uri.")
		return
	}
	sess, ok := o.sessions.sessionFromRequest(r)
	if !ok {
		renderLogin(w, "/authorize?"+authorizeQuery(r.PostForm), "Sitzung abgelaufen — bitte erneut anmelden.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(sess.csrf)) != 1 {
		renderError(w, http.StatusForbidden, "CSRF-Prüfung fehlgeschlagen.")
		return
	}
	if !sameOrigin(r, o.cfg.issuer) {
		renderError(w, http.StatusForbidden, "Ungültige Herkunft (Origin).")
		return
	}
	if r.PostForm.Get("action") != "approve" {
		redirectError(w, redirectURI, state, "access_denied", "the user denied the request")
		return
	}
	challenge := r.PostForm.Get("code_challenge")
	if challenge == "" {
		redirectError(w, redirectURI, state, "invalid_request", "missing code_challenge")
		return
	}
	scopes := o.effectiveScopes(sess, r.PostForm.Get("scope"))
	if len(scopes) == 0 {
		redirectError(w, redirectURI, state, "invalid_scope", "the signed-in key cannot grant any issuable scope")
		return
	}
	code, err := randToken(32)
	if err != nil {
		redirectError(w, redirectURI, state, "server_error", "could not allocate code")
		return
	}
	o.mu.Lock()
	o.codes[code] = &authCode{
		clientID:    clientID,
		redirectURI: redirectURI,
		challenge:   challenge,
		resource:    r.PostForm.Get("resource"),
		scopes:      scopes,
		subject:     sess.subject,
		expires:     o.now().Add(authCodeTTL),
	}
	o.mu.Unlock()

	u, _ := url.Parse(redirectURI)
	rq := u.Query()
	rq.Set("code", code)
	if state != "" {
		rq.Set("state", state)
	}
	u.RawQuery = rq.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// --- login / logout -------------------------------------------------------

// handleLogin establishes a cookie session from a kid.secret. It is the human
// authentication step behind /authorize; on success it redirects to `next`
// (always a local /authorize URL) so consent can proceed.
func (o *oauthServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, "Ungültiges Formular.")
		return
	}
	next := r.PostForm.Get("next")
	if !strings.HasPrefix(next, "/authorize?") {
		next = "" // never redirect off-path
	}
	key, ok := o.ks.authenticate(r.PostForm.Get("bearer"))
	if !ok {
		renderLogin(w, next, "Ungültiger Schlüssel.")
		return
	}
	sess, err := o.sessions.create(key.Kid, key.Scopes)
	if err != nil {
		renderError(w, http.StatusInternalServerError, "Sitzung konnte nicht angelegt werden.")
		return
	}
	setSessionCookie(w, sess.id, o.cfg.secure, o.sessions.ttl)
	if next == "" {
		renderError(w, http.StatusOK, "Angemeldet. Diese Seite kann geschlossen werden.")
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (o *oauthServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		o.sessions.destroy(c.Value)
	}
	clearSessionCookie(w, o.cfg.secure)
	w.WriteHeader(http.StatusNoContent)
}

// --- token endpoint -------------------------------------------------------

// handleToken exchanges an authorization code (with PKCE verifier) or a refresh
// token for a fresh access token — a short-lived managed keystore key.
func (o *oauthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthJSONError(w, http.StatusBadRequest, "invalid_request", "cannot parse form")
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		o.tokenFromCode(w, r)
	case "refresh_token":
		o.tokenFromRefresh(w, r)
	default:
		oauthJSONError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

func (o *oauthServer) tokenFromCode(w http.ResponseWriter, r *http.Request) {
	code := r.PostForm.Get("code")
	o.mu.Lock()
	ac, ok := o.codes[code]
	if ok && (ac.used || o.now().After(ac.expires)) {
		delete(o.codes, code)
		ok = false
	}
	if ok {
		ac.used = true
		delete(o.codes, code)
	}
	o.mu.Unlock()
	if !ok {
		oauthJSONError(w, http.StatusBadRequest, "invalid_grant", "unknown, used or expired code")
		return
	}
	if r.PostForm.Get("redirect_uri") != ac.redirectURI || r.PostForm.Get("client_id") != ac.clientID {
		oauthJSONError(w, http.StatusBadRequest, "invalid_grant", "client_id/redirect_uri mismatch")
		return
	}
	if !verifyPKCE(r.PostForm.Get("code_verifier"), ac.challenge) {
		oauthJSONError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}
	if !o.resourceValid(r.PostForm.Get("resource"), ac.resource) {
		oauthJSONError(w, http.StatusBadRequest, "invalid_target", "resource does not identify this server")
		return
	}
	o.issueToken(w, ac.clientID, ac.subject, ac.scopes, ac.resource)
}

func (o *oauthServer) tokenFromRefresh(w http.ResponseWriter, r *http.Request) {
	rt := r.PostForm.Get("refresh_token")
	o.mu.Lock()
	rg, ok := o.refresh[rt]
	if ok {
		delete(o.refresh, rt) // rotate: the old refresh token is spent
	}
	o.mu.Unlock()
	if !ok {
		oauthJSONError(w, http.StatusBadRequest, "invalid_grant", "unknown or spent refresh token")
		return
	}
	if cid := r.PostForm.Get("client_id"); cid != "" && cid != rg.clientID {
		oauthJSONError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}
	o.issueToken(w, rg.clientID, rg.subject, rg.scopes, rg.resource)
}

// issueToken mints a managed keystore key as the access token and a rotating
// refresh token, and writes the OAuth token response.
func (o *oauthServer) issueToken(w http.ResponseWriter, clientID, subject string, scopes []Scope, resource string) {
	kid, secret, err := o.ks.createKey(scopes, "oauth:"+clientID, o.now().Add(o.cfg.accessTTL))
	if err != nil {
		oauthJSONError(w, http.StatusInternalServerError, "server_error", "could not mint token")
		return
	}
	rt, err := randToken(32)
	if err != nil {
		oauthJSONError(w, http.StatusInternalServerError, "server_error", "could not mint refresh token")
		return
	}
	o.mu.Lock()
	o.refresh[rt] = &refreshGrant{clientID: clientID, scopes: scopes, subject: subject, resource: resource}
	o.mu.Unlock()
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"access_token":  kid + "." + secret,
		"token_type":    "Bearer",
		"expires_in":    int(o.cfg.accessTTL.Seconds()),
		"refresh_token": rt,
		"scope":         strings.Join(scopeStrings(scopes), " "),
	})
}

// --- helpers --------------------------------------------------------------

// effectiveScopes computes the scopes a token may carry: the intersection of the
// issuable set (cfg.grantScopes), any explicitly requested scopes, and what the
// signed-in key can actually grant (admin covers all).
func (o *oauthServer) effectiveScopes(sess *session, requested string) []Scope {
	want := map[Scope]bool{}
	if req := strings.Fields(requested); len(req) > 0 {
		for _, s := range req {
			want[Scope(s)] = true
		}
	}
	granter := &Key{Scopes: sess.scopes}
	var out []Scope
	for _, s := range o.cfg.grantScopes {
		if len(want) > 0 && !want[s] {
			continue
		}
		if granter.HasScope(s) {
			out = append(out, s)
		}
	}
	return out
}

// redirectAllowed reports whether redirectURI is acceptable for clientID: an
// exact match against a registered client's URIs, or (for unregistered clients
// such as the connector's fixed id) a host on the allowlist.
func (o *oauthServer) redirectAllowed(clientID, redirectURI string) bool {
	o.mu.Lock()
	c, registered := o.clients[clientID]
	o.mu.Unlock()
	if registered {
		for _, u := range c.redirectURIs {
			if u == redirectURI {
				return true
			}
		}
		return false
	}
	return o.redirectHostAllowed(redirectURI)
}

// redirectHostAllowed validates a redirect target by scheme and host: https for
// allowlisted hosts, or http only for loopback.
func (o *oauthServer) redirectHostAllowed(redirectURI string) bool {
	u, err := url.Parse(redirectURI)
	if err != nil || u.Host == "" {
		return false
	}
	host := u.Hostname()
	loopback := host == "localhost" || host == "127.0.0.1"
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return false
	}
	for _, h := range o.cfg.redirectHosts {
		if strings.EqualFold(host, h) {
			return true
		}
	}
	return false
}

// resourceValid checks the token request's resource against the code's resource
// and this server's identity (RFC 8707). An empty resource is accepted (lenient
// for clients that omit it); a present one must name this server's host.
func (o *oauthServer) resourceValid(reqResource, codeResource string) bool {
	res := reqResource
	if res == "" {
		res = codeResource
	}
	if res == "" {
		return true
	}
	ru, err := url.Parse(res)
	if err != nil {
		return false
	}
	iu, err := url.Parse(o.cfg.issuer)
	if err != nil {
		return false
	}
	return strings.EqualFold(ru.Hostname(), iu.Hostname())
}

// verifyPKCE checks base64url(sha256(verifier)) == challenge (S256), in constant
// time. A missing verifier or challenge fails.
func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// authorizeQuery rebuilds the /authorize query string from a consent POST's
// fields, so a re-login can return to the same request.
func authorizeQuery(form url.Values) string {
	q := url.Values{}
	q.Set("response_type", "code")
	for _, k := range []string{"client_id", "redirect_uri", "state", "code_challenge", "resource", "scope"} {
		if v := form.Get(k); v != "" {
			q.Set(k, v)
		}
	}
	q.Set("code_challenge_method", "S256")
	return q.Encode()
}

// sameOrigin checks the request's Origin (or Referer) matches the issuer host,
// a second CSRF guard on top of the per-session token.
func sameOrigin(r *http.Request, issuer string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true // no Origin/Referer (some clients omit it); rely on the CSRF token
	}
	ou, err := url.Parse(origin)
	if err != nil {
		return false
	}
	iu, err := url.Parse(issuer)
	if err != nil {
		return false
	}
	return strings.EqualFold(ou.Hostname(), iu.Hostname())
}

// writeOAuthJSON writes a JSON body with no-store caching, as OAuth token and
// metadata responses require. It reuses the shared writeJSON encoder.
func writeOAuthJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, body)
}

func oauthJSONError(w http.ResponseWriter, status int, code, desc string) {
	writeOAuthJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func redirectError(w http.ResponseWriter, redirectURI, state, code, desc string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		renderError(w, http.StatusBadRequest, "Ungültige redirect_uri.")
		return
	}
	q := u.Query()
	q.Set("error", code)
	q.Set("error_description", desc)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	// A ResponseWriter with no request handle: emit a minimal redirect ourselves.
	w.Header().Set("Location", u.String())
	w.WriteHeader(http.StatusFound)
}

// --- server-rendered pages ------------------------------------------------

type consentData struct {
	CSRF        string
	ClientID    string
	RedirectURI string
	State       string
	Challenge   string
	Resource    string
	Scope       string
	Subject     string
	Scopes      []string
}

var pageStyle = `<style>
  body{font-family:system-ui,sans-serif;background:#0f1117;color:#e6e8ee;display:flex;
       min-height:100vh;align-items:center;justify-content:center;margin:0}
  .card{background:#171a23;border:1px solid #262b38;border-radius:12px;padding:28px 32px;max-width:440px;width:90%}
  h1{font-size:1.15rem;margin:0 0 4px} p{color:#9aa3b2;font-size:.9rem;line-height:1.5}
  code{background:#0f1117;padding:1px 6px;border-radius:5px;color:#8ab4ff}
  input{width:100%;box-sizing:border-box;background:#0f1117;border:1px solid #2b3242;color:#e6e8ee;
        border-radius:8px;padding:10px 12px;font-size:.95rem;margin:6px 0 14px}
  button{border:0;border-radius:8px;padding:10px 16px;font-size:.95rem;font-weight:600;cursor:pointer}
  .primary{background:#3b6cff;color:#fff} .ghost{background:#20263300;color:#9aa3b2;border:1px solid #2b3242}
  ul{margin:8px 0 16px;padding-left:18px} li{color:#c9cfdb;font-size:.88rem;margin:2px 0}
  .row{display:flex;gap:10px;justify-content:flex-end} .err{color:#ff8a8a;font-size:.85rem;margin:0 0 10px}
</style>`

var loginTmpl = template.Must(template.New("login").Parse(`<!doctype html><html><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>temis — Anmelden</title>` + pageStyle + `</head><body><div class="card">
<h1>Bei temis anmelden</h1>
<p>Ein MCP-Client möchte auf temis zugreifen. Melde dich mit deinem API-Schlüssel
(<code>kid.secret</code>) an, um fortzufahren.</p>
{{if .Err}}<p class="err">{{.Err}}</p>{{end}}
<form method="post" action="/oauth/login">
  <input type="password" name="bearer" placeholder="kid.secret" autocomplete="off" autofocus>
  <input type="hidden" name="next" value="{{.Next}}">
  <div class="row"><button class="primary" type="submit">Anmelden</button></div>
</form></div></body></html>`))

var consentTmpl = template.Must(template.New("consent").Parse(`<!doctype html><html><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>temis — Zugriff erlauben</title>` + pageStyle + `</head><body><div class="card">
<h1>Zugriff erlauben?</h1>
<p>Client <code>{{.ClientID}}</code> möchte im Namen von <code>{{.Subject}}</code>
ein Token mit diesen Rechten:</p>
<ul>{{range .Scopes}}<li>{{.}}</li>{{end}}</ul>
<form method="post" action="/authorize">
  <input type="hidden" name="csrf" value="{{.CSRF}}">
  <input type="hidden" name="client_id" value="{{.ClientID}}">
  <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
  <input type="hidden" name="state" value="{{.State}}">
  <input type="hidden" name="code_challenge" value="{{.Challenge}}">
  <input type="hidden" name="resource" value="{{.Resource}}">
  <input type="hidden" name="scope" value="{{.Scope}}">
  <div class="row">
    <button class="ghost" type="submit" name="action" value="deny">Ablehnen</button>
    <button class="primary" type="submit" name="action" value="approve">Erlauben</button>
  </div>
</form></div></body></html>`))

var errorTmpl = template.Must(template.New("err").Parse(`<!doctype html><html><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>temis</title>` + pageStyle + `</head><body><div class="card">
<h1>temis</h1><p>{{.}}</p></div></body></html>`))

func renderLogin(w http.ResponseWriter, next, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = loginTmpl.Execute(w, map[string]any{"Next": next, "Err": errMsg})
}

func renderConsent(w http.ResponseWriter, d consentData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = consentTmpl.Execute(w, d)
}

func renderError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = errorTmpl.Execute(w, msg)
}
