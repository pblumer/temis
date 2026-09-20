package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/mcp"
)

// s256 is the PKCE S256 transform: base64url(sha256(verifier)), no padding.
func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// newOAuthTestServer builds an httptest server with OAuth enabled: a static admin
// key to log in with, a managed keystore (so tokens can be minted) and a
// co-located /mcp endpoint to prove the issued token works end to end.
func newOAuthTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	keys := writeKeysFile(t, []scopedKey{{"admin", "adminsecret", []Scope{ScopeAdmin}}})
	srv := NewServer(nil,
		WithKeysFile(keys),
		WithKeyStore(t.TempDir()),
		WithExternalURL("https://temis.test"),
	)
	srv.AttachMCP(mcp.NewServer(dmn.New(), mcp.WithAuth(srv.MCPAuth()), mcp.WithStore(srv.ModelStore())))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, "admin.adminsecret"
}

// noRedirectClient returns responses verbatim (never following 3xx), so tests can
// inspect Location and Set-Cookie on the redirect steps of the OAuth flow.
func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

var csrfRe = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func TestOAuthMetadata(t *testing.T) {
	ts, _ := newOAuthTestServer(t)
	resp, err := http.Get(ts.URL + "/.well-known/oauth-authorization-server")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("metadata status = %d, want 200", resp.StatusCode)
	}
	var md map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&md); err != nil {
		t.Fatal(err)
	}
	if md["issuer"] != "https://temis.test" {
		t.Errorf("issuer = %v, want https://temis.test", md["issuer"])
	}
	if md["authorization_endpoint"] != "https://temis.test/authorize" {
		t.Errorf("authorization_endpoint = %v", md["authorization_endpoint"])
	}
	methods, _ := md["code_challenge_methods_supported"].([]any)
	if len(methods) != 1 || methods[0] != "S256" {
		t.Errorf("code_challenge_methods_supported = %v, want [S256]", md["code_challenge_methods_supported"])
	}

	// Protected-resource metadata points back at this same server as the AS.
	resp2, err := http.Get(ts.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()
	var prm map[string]any
	_ = json.NewDecoder(resp2.Body).Decode(&prm)
	if prm["resource"] != "https://temis.test" {
		t.Errorf("resource = %v", prm["resource"])
	}
}

func TestOAuthFullFlow(t *testing.T) {
	ts, adminKey := newOAuthTestServer(t)
	client := noRedirectClient()

	verifier := "a-sufficiently-long-code-verifier-value-1234567890"
	redirectURI := "https://claude.ai/api/mcp/auth_callback"
	authzQuery := url.Values{
		"response_type":         {"code"},
		"client_id":             {"claude-code-01"},
		"redirect_uri":          {redirectURI},
		"state":                 {"xyz-state"},
		"code_challenge":        {s256(verifier)},
		"code_challenge_method": {"S256"},
		"resource":              {"https://temis.test"},
	}.Encode()

	// 1. /authorize without a session → the login page.
	resp, err := client.Get(ts.URL + "/authorize?" + authzQuery)
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != 200 || !strings.Contains(body, "Anmelden") {
		t.Fatalf("expected login page, got status=%d body=%.120s", resp.StatusCode, body)
	}

	// 2. Log in with the admin key → session cookie + redirect back to /authorize.
	form := url.Values{"bearer": {adminKey}, "next": {"/authorize?" + authzQuery}}
	resp, err = client.PostForm(ts.URL+"/oauth/login", form)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303", resp.StatusCode)
	}
	cookie := sessionCookie(t, resp)

	// 3. /authorize with the session cookie → consent page (carries the CSRF token).
	req, _ := http.NewRequest("GET", ts.URL+"/authorize?"+authzQuery, nil)
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	consent := readAll(t, resp)
	if resp.StatusCode != 200 || !strings.Contains(consent, "Erlauben") {
		t.Fatalf("expected consent page, got status=%d body=%.120s", resp.StatusCode, consent)
	}
	m := csrfRe.FindStringSubmatch(consent)
	if m == nil {
		t.Fatal("no csrf token in consent page")
	}
	csrf := m[1]

	// 4. Approve → redirect to the client with an authorization code.
	approve := url.Values{
		"csrf": {csrf}, "action": {"approve"},
		"client_id": {"claude-code-01"}, "redirect_uri": {redirectURI},
		"state": {"xyz-state"}, "code_challenge": {s256(verifier)},
		"resource": {"https://temis.test"},
	}
	req, _ = http.NewRequest("POST", ts.URL+"/authorize", strings.NewReader(approve.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("approve status = %d, want 302", resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Query().Get("state") != "xyz-state" {
		t.Errorf("state not echoed: %q", loc.Query().Get("state"))
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect")
	}

	// 5. Exchange the code (with the PKCE verifier) for an access token.
	tok := exchangeCode(t, ts, client, code, verifier, redirectURI)
	if tok.TokenType != "Bearer" || tok.AccessToken == "" || tok.RefreshToken == "" {
		t.Fatalf("bad token response: %+v", tok)
	}

	// 6. The access token is a real bearer for the co-located /mcp endpoint.
	if st := callMCPListModels(t, ts, tok.AccessToken); st != 200 {
		t.Fatalf("/mcp with issued token = %d, want 200", st)
	}
	// A garbage token is rejected.
	if st := callMCPListModels(t, ts, "kid.bogus"); st != 401 {
		t.Fatalf("/mcp with bad token = %d, want 401", st)
	}

	// 7. The code is single-use: a second exchange fails.
	if st, _ := postToken(t, ts, client, url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"code_verifier": {verifier}, "redirect_uri": {redirectURI}, "client_id": {"claude-code-01"},
	}); st != 400 {
		t.Fatalf("code reuse status = %d, want 400", st)
	}

	// 8. Refresh yields a fresh working token.
	st, refreshed := postToken(t, ts, client, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {tok.RefreshToken}, "client_id": {"claude-code-01"},
	})
	if st != 200 {
		t.Fatalf("refresh status = %d, want 200", st)
	}
	if callMCPListModels(t, ts, refreshed.AccessToken) != 200 {
		t.Fatal("refreshed token did not work on /mcp")
	}
}

func TestOAuthPKCEPlainRejected(t *testing.T) {
	ts, _ := newOAuthTestServer(t)
	client := noRedirectClient()
	q := url.Values{
		"response_type": {"code"}, "client_id": {"c"},
		"redirect_uri":   {"https://claude.ai/api/mcp/auth_callback"},
		"code_challenge": {"abc"}, "code_challenge_method": {"plain"},
	}.Encode()
	resp, err := client.Get(ts.URL + "/authorize?" + q)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	// A protocol error redirects back with error=invalid_request (not the login page).
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 error redirect", resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Query().Get("error") != "invalid_request" {
		t.Errorf("error = %q, want invalid_request", loc.Query().Get("error"))
	}
}

func TestOAuthRejectsBadRedirect(t *testing.T) {
	ts, _ := newOAuthTestServer(t)
	client := noRedirectClient()
	q := url.Values{
		"response_type": {"code"}, "client_id": {"c"},
		"redirect_uri":   {"https://evil.example.com/callback"},
		"code_challenge": {"abc"}, "code_challenge_method": {"S256"},
	}.Encode()
	resp, err := client.Get(ts.URL + "/authorize?" + q)
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(t, resp)
	// A disallowed redirect must NOT redirect (open-redirect guard) — plain 400.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (got body %.80s)", resp.StatusCode, body)
	}
	if resp.Header.Get("Location") != "" {
		t.Error("must not emit a Location for a disallowed redirect_uri")
	}
}

func TestOAuthReaper(t *testing.T) {
	ks := newKeystore()
	o := newOAuthServer(oauthConfig{issuer: "https://temis.test"}, ks, newSessionStore(time.Hour))
	now := time.Unix(1_700_000_000, 0)
	o.now = func() time.Time { return now }
	ks.now = func() time.Time { return now }

	// One short-lived issued token key, one long-lived key, one never-expiring
	// unmanaged key; plus a pending code and a refresh grant.
	shortKid, _, err := ks.createKey([]Scope{ScopeEvaluate}, "oauth:test", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	longKid, _, err := ks.createKey([]Scope{ScopeEvaluate}, "oauth:test", now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.add(&Key{Kid: "static", Scopes: []Scope{ScopeAdmin}}); err != nil {
		t.Fatal(err)
	}
	o.codes["c1"] = &authCode{expires: now.Add(authCodeTTL)}
	o.refresh["r1"] = &refreshGrant{expires: now.Add(defaultRefreshTTL)}

	// Nothing has aged out yet.
	if c, r, k := o.reap(now); c+r+k != 0 {
		t.Fatalf("premature reap: codes=%d refresh=%d keys=%d", c, r, k)
	}

	// Advance two hours: the short-lived key and the code expire; the refresh grant
	// and the long-lived + static keys survive.
	now = now.Add(2 * time.Hour)
	c, r, k := o.reap(now)
	if c != 1 || r != 0 || k != 1 {
		t.Fatalf("reap = codes:%d refresh:%d keys:%d, want 1/0/1", c, r, k)
	}
	if _, ok := ks.keys[shortKid]; ok {
		t.Error("expired access-token key was not reaped")
	}
	if _, ok := ks.keys[longKid]; !ok {
		t.Error("unexpired key must survive")
	}
	if _, ok := ks.keys["static"]; !ok {
		t.Error("unmanaged never-expiring key must survive")
	}

	// Far future: the long-lived key and the refresh grant age out too.
	now = now.Add(defaultRefreshTTL + time.Hour)
	c, r, k = o.reap(now)
	if r != 1 || k != 1 {
		t.Fatalf("late reap = codes:%d refresh:%d keys:%d, want refresh 1 keys 1", c, r, k)
	}
	if _, ok := ks.keys["static"]; !ok {
		t.Error("unmanaged key must still survive")
	}
}

// --- helpers --------------------------------------------------------------

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func sessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatal("no session cookie set by login")
	return nil
}

func postToken(t *testing.T, ts *httptest.Server, client *http.Client, form url.Values) (int, tokenResp) {
	t.Helper()
	resp, err := client.PostForm(ts.URL+"/token", form)
	if err != nil {
		t.Fatal(err)
	}
	var tr tokenResp
	_ = json.Unmarshal([]byte(readAll(t, resp)), &tr)
	return resp.StatusCode, tr
}

func exchangeCode(t *testing.T, ts *httptest.Server, client *http.Client, code, verifier, redirectURI string) tokenResp {
	t.Helper()
	st, tr := postToken(t, ts, client, url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"code_verifier": {verifier}, "redirect_uri": {redirectURI}, "client_id": {"claude-code-01"},
	})
	if st != 200 {
		t.Fatalf("token exchange status = %d (%s)", st, tr.Error)
	}
	return tr
}

func callMCPListModels(t *testing.T, ts *httptest.Server, bearer string) int {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models","arguments":{}}}`
	req, _ := http.NewRequest("POST", ts.URL+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}
