package service

import (
	"net/http"
	"testing"
)

// TestOwnershipVisibility covers the owner visibility rules (WP-106): the owning
// key sees its resource, another key does not, and an admin (seeAll) or an unowned
// resource is visible to everyone.
func TestOwnershipVisibility(t *testing.T) {
	o, err := newOwnership(nil)
	if err != nil {
		t.Fatalf("newOwnership: %v", err)
	}
	const id = "sha256:aa"
	if err := o.claim(id, authIdentity{kid: "alice"}); err != nil {
		t.Fatalf("claim: %v", err)
	}

	cases := []struct {
		name  string
		ident authIdentity
		want  bool
	}{
		{"owner sees own", authIdentity{kid: "alice"}, true},
		{"stranger blocked", authIdentity{kid: "bob"}, false},
		{"admin sees all", authIdentity{kid: "boss", seeAll: true}, true},
	}
	for _, tc := range cases {
		if got := o.visible(id, tc.ident); got != tc.want {
			t.Errorf("%s: visible = %v, want %v", tc.name, got, tc.want)
		}
	}

	// An unclaimed id is unowned → visible to everyone.
	if !o.visible("sha256:unknown", authIdentity{kid: "bob"}) {
		t.Error("unowned resource should be visible to all")
	}

	// ownedByKid is the narrower "mine" check.
	if !o.ownedByKid(id, authIdentity{kid: "alice"}) {
		t.Error("ownedByKid(alice) = false, want true")
	}
	if o.ownedByKid(id, authIdentity{kid: "bob"}) {
		t.Error("ownedByKid(bob) = true, want false")
	}
}

// TestOwnershipClaimNoOpWithoutIdentity asserts an unauthenticated write (empty
// kid) claims nothing, so the resource stays unowned and shared — the open-API
// default (WP-106).
func TestOwnershipClaimNoOpWithoutIdentity(t *testing.T) {
	o, _ := newOwnership(nil)
	if err := o.claim("sha256:x", authIdentity{}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(o.owners) != 0 {
		t.Fatalf("empty-identity claim recorded an owner: %v", o.owners)
	}
	if !o.visible("sha256:x", authIdentity{kid: "carol"}) {
		t.Error("unclaimed resource should stay visible to all")
	}
}

// TestOwnershipPersistsAcrossReload asserts the ownership index round-trips
// through its on-disk form, so owner isolation survives a restart (WP-106).
func TestOwnershipPersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	persist := newOwnerPersist(dir)
	o1, err := newOwnership(persist)
	if err != nil {
		t.Fatalf("newOwnership: %v", err)
	}
	if err := o1.claim("sha256:m", authIdentity{kid: "alice"}); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// A fresh index over the same directory reloads the claim.
	o2, err := newOwnership(newOwnerPersist(dir))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !o2.visible("sha256:m", authIdentity{kid: "alice"}) {
		t.Error("reloaded index lost the owner claim")
	}
	if o2.visible("sha256:m", authIdentity{kid: "bob"}) {
		t.Error("reloaded index leaked the model to another key")
	}
}

// TestHTTPOwnerIsolation is the WP-106 acceptance test: a model created by one key
// is visible to its creator and an admin, but returns 404 to another key (hard
// isolation, not a mere list filter), and the catalog listing filters accordingly
// — including "?owner=me" for just-mine (excluding shared examples).
func TestHTTPOwnerIsolation(t *testing.T) {
	scopes := []Scope{ScopeModelsRead, ScopeModelsWrite, ScopeFlow}
	path := writeKeysFile(t, []scopedKey{
		{kid: "alice", secret: "a", scopes: scopes},
		{kid: "carol", secret: "c", scopes: scopes},
		{kid: "boss", secret: "z", scopes: []Scope{ScopeAdmin}},
	})
	h := NewServer(nil, WithKeysFile(path)).Handler()

	// Alice creates a model; it is claimed for her.
	rec := doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), "alice.a")
	if rec.Code != http.StatusCreated {
		t.Fatalf("alice create = %d (%s)", rec.Code, rec.Body)
	}
	id := decode[modelResponse](t, rec).ModelID

	// Direct GET on the model id.
	getCases := []struct {
		bearer string
		want   int
	}{
		{"alice.a", http.StatusOK},       // owner
		{"boss.z", http.StatusOK},        // admin sees all
		{"carol.c", http.StatusNotFound}, // another key → 404 (hard isolation)
	}
	for _, tc := range getCases {
		if rec := doAuth(t, h, "GET", "/v1/models/"+id, "", nil, tc.bearer); rec.Code != tc.want {
			t.Errorf("GET model as %s = %d, want %d (%s)", tc.bearer, rec.Code, tc.want, rec.Body)
		}
	}

	// Catalog listing is filtered by identity.
	hasModel := func(bearer, query string) bool {
		t.Helper()
		rec := doAuth(t, h, "GET", "/v1/models"+query, "", nil, bearer)
		if rec.Code != http.StatusOK {
			t.Fatalf("list as %s = %d (%s)", bearer, rec.Code, rec.Body)
		}
		for _, m := range decode[modelListResponse](t, rec).Models {
			if m.ModelID == id {
				return true
			}
		}
		return false
	}
	if !hasModel("alice.a", "") {
		t.Error("alice's own model missing from her listing")
	}
	if hasModel("carol.c", "") {
		t.Error("another key carol can list the model")
	}
	if !hasModel("alice.a", "?owner=me") {
		t.Error("alice's own model missing from owner=me listing")
	}
}

// TestHTTPOwnerIsolationFlows asserts the same isolation covers registered
// decision flows (WP-106).
func TestHTTPOwnerIsolationFlows(t *testing.T) {
	path := writeKeysFile(t, []scopedKey{
		{kid: "alice", secret: "a", scopes: []Scope{ScopeFlow}},
		{kid: "carol", secret: "c", scopes: []Scope{ScopeFlow}},
	})
	h := NewServer(nil, WithKeysFile(path)).Handler()

	rec := doAuth(t, h, "POST", "/v1/flows", "application/json", []byte(`{"name":"x","steps":[]}`), "alice.a")
	if rec.Code != http.StatusCreated {
		t.Fatalf("alice create flow = %d (%s)", rec.Code, rec.Body)
	}
	id := decode[flowResponse](t, rec).FlowID

	if rec := doAuth(t, h, "GET", "/v1/flows/"+id, "", nil, "alice.a"); rec.Code != http.StatusOK {
		t.Errorf("owner GET flow = %d, want 200", rec.Code)
	}
	if rec := doAuth(t, h, "GET", "/v1/flows/"+id, "", nil, "carol.c"); rec.Code != http.StatusNotFound {
		t.Errorf("other key GET flow = %d, want 404", rec.Code)
	}
}

// TestHTTPExampleModelsStayShared asserts models seeded without an authenticated
// creator (the bundled examples) are unowned and therefore visible to every key,
// so isolation never hides the shared starting set (WP-106).
func TestHTTPExampleModelsStayShared(t *testing.T) {
	path := writeKeysFile(t, []scopedKey{
		{kid: "carol", secret: "c", scopes: []Scope{ScopeModelsRead}},
	})
	h := NewServer(nil, WithExamples(), WithKeysFile(path)).Handler()

	rec := doAuth(t, h, "GET", "/v1/models", "", nil, "carol.c")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d (%s)", rec.Code, rec.Body)
	}
	list := decode[modelListResponse](t, rec)
	if list.Count == 0 {
		t.Fatal("expected bundled example models to be visible to any key")
	}
	// And a direct GET on an example id also passes (unowned → shared).
	if rec := doAuth(t, h, "GET", "/v1/models/"+list.Models[0].ModelID, "", nil, "carol.c"); rec.Code != http.StatusOK {
		t.Errorf("GET example model = %d, want 200", rec.Code)
	}
}
