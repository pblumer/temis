package service

import (
	"net/http"
	"sync"
	"time"
)

// sessionCookieName is the browser cookie that carries the opaque server-side
// session id for the OAuth consent surface (ADR-0038). It is HttpOnly (never
// readable from JS), Secure when the server runs behind TLS, and SameSite=Lax so
// it still rides the top-level navigation from the OAuth client (claude.ai) back
// to GET /authorize while staying off cross-site subrequests.
const sessionCookieName = "temis_session"

// defaultSessionTTL bounds how long a human login stays valid before /authorize
// asks for the kid.secret again. Sessions live in memory only; a restart logs
// everyone out (acceptable — the OAuth access tokens they minted are managed
// keys that survive independently).
const defaultSessionTTL = 12 * time.Hour

// session is one authenticated human login. subject is the kid of the key that
// proved identity; scopes is that key's grant, the ceiling for any token minted
// during the session. csrf guards the consent POST against cross-site forgery.
type session struct {
	id      string
	subject string
	scopes  []Scope
	csrf    string
	expires time.Time
}

// sessionStore is the in-memory set of live login sessions, keyed by their
// random 256-bit id. It is mutex-guarded like the keystore and prunes lazily on
// lookup, so an abandoned session simply expires.
type sessionStore struct {
	mu   sync.Mutex
	byID map[string]*session
	ttl  time.Duration
	now  func() time.Time
}

func newSessionStore(ttl time.Duration) *sessionStore {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	return &sessionStore{byID: map[string]*session{}, ttl: ttl, now: time.Now}
}

// create records a new session for the identity subject with the given scope
// ceiling and returns it. The id and csrf token are fresh 256-bit random hex.
func (ss *sessionStore) create(subject string, scopes []Scope) (*session, error) {
	id, err := randToken(32)
	if err != nil {
		return nil, err
	}
	csrf, err := randToken(32)
	if err != nil {
		return nil, err
	}
	sess := &session{
		id:      id,
		subject: subject,
		scopes:  scopes,
		csrf:    csrf,
		expires: ss.now().Add(ss.ttl),
	}
	ss.mu.Lock()
	ss.byID[id] = sess
	ss.mu.Unlock()
	return sess, nil
}

// get resolves a session id to a live (unexpired) session. An expired session is
// pruned and reported as absent.
func (ss *sessionStore) get(id string) (*session, bool) {
	if id == "" {
		return nil, false
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	sess, ok := ss.byID[id]
	if !ok {
		return nil, false
	}
	if ss.now().After(sess.expires) {
		delete(ss.byID, id)
		return nil, false
	}
	return sess, true
}

// destroy removes a session (logout). Removing an unknown id is a no-op.
func (ss *sessionStore) destroy(id string) {
	if id == "" {
		return
	}
	ss.mu.Lock()
	delete(ss.byID, id)
	ss.mu.Unlock()
}

// sessionFromRequest returns the live session named by the request's session
// cookie, if any.
func (ss *sessionStore) sessionFromRequest(r *http.Request) (*session, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, false
	}
	return ss.get(c.Value)
}

// setSessionCookie writes the session cookie. secure marks it Secure so it is
// only sent over HTTPS; callers pass whether the server terminates TLS.
func setSessionCookie(w http.ResponseWriter, id string, secure bool, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

// clearSessionCookie expires the session cookie in the client.
func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
