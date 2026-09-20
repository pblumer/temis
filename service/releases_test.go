package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.9.9", 1},
		{"1.2.0", "1.10.0", -1}, // numeric, not lexical
		{"2.1", "2.1.0", 0},     // missing component counts as 0
		{"v3", "v2", 1},
		{"1.0.0", "1.0.0-rc.1", 1}, // release outranks its pre-release
		{"1.0.0-rc.1", "1.0.0-rc.2", -1},
		{"v1", "1", 0}, // leading v ignored
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := compareVersions(c.b, c.a); got != -c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d (antisymmetry)", c.b, c.a, got, -c.want)
		}
	}
}

const (
	idA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	idC = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func fixedClock() func() time.Time {
	ts := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return ts }
}

func TestReleaseStorePublishAndResolve(t *testing.T) {
	rs, err := newReleaseStore("")
	if err != nil {
		t.Fatal(err)
	}
	rs.now = fixedClock()

	if _, err := rs.publish("Pricing", "1.0.0", idA, "first"); err != nil {
		t.Fatalf("publish 1.0.0: %v", err)
	}
	if _, err := rs.publish("Pricing", "2.1.0", idB, ""); err != nil {
		t.Fatalf("publish 2.1.0: %v", err)
	}

	// Publishing the same (name, version) again is a conflict.
	if _, err := rs.publish("Pricing", "1.0.0", idC, ""); err != errReleaseExists {
		t.Errorf("re-publish err = %v, want errReleaseExists", err)
	}

	// latest tracks the highest version automatically.
	if mid, ok := rs.resolve("Pricing@latest"); !ok || mid != idB {
		t.Errorf("resolve latest = %q,%v, want %q,true", mid, ok, idB)
	}
	// A bare name resolves via latest.
	if mid, ok := rs.resolve("Pricing"); !ok || mid != idB {
		t.Errorf("resolve bare = %q,%v, want %q,true", mid, ok, idB)
	}
	// An explicit version pins that exact revision.
	if mid, ok := rs.resolve("Pricing@1.0.0"); !ok || mid != idA {
		t.Errorf("resolve 1.0.0 = %q,%v, want %q,true", mid, ok, idA)
	}
	// A raw model id is not a release reference (caller handles it directly).
	if _, ok := rs.resolve(idA); ok {
		t.Errorf("resolve of raw model id reported ok, want false")
	}
	// Unknown name/version resolve to nothing.
	if _, ok := rs.resolve("Nope@9.9.9"); ok {
		t.Errorf("resolve unknown reported ok, want false")
	}
	if _, ok := rs.resolve("Pricing@3.0.0"); ok {
		t.Errorf("resolve missing version reported ok, want false")
	}

	// Newest-first ordering.
	mr, ok := rs.get("Pricing")
	if !ok || len(mr.Releases) != 2 || mr.Releases[0].Version != "2.1.0" {
		t.Fatalf("get Pricing = %+v (ok=%v), want 2 releases newest-first", mr, ok)
	}
	if mr.Releases[0].PublishedAt != "2026-07-21T12:00:00Z" {
		t.Errorf("publishedAt = %q, want the fixed clock's time", mr.Releases[0].PublishedAt)
	}
}

func TestReleaseStoreChannels(t *testing.T) {
	rs, _ := newReleaseStore("")
	rs.now = fixedClock()
	_, _ = rs.publish("Risk", "1.0.0", idA, "")
	_, _ = rs.publish("Risk", "2.0.0", idB, "")

	// stable stays on 1.0.0 while latest moved to 2.0.0.
	if err := rs.setChannel("Risk", "stable", "1.0.0"); err != nil {
		t.Fatalf("setChannel: %v", err)
	}
	if mid, ok := rs.resolve("Risk@stable"); !ok || mid != idA {
		t.Errorf("resolve stable = %q,%v, want %q", mid, ok, idA)
	}
	if mid, _ := rs.resolve("Risk@latest"); mid != idB {
		t.Errorf("resolve latest = %q, want %q", mid, idB)
	}

	// A channel on an unpublished version is rejected.
	if err := rs.setChannel("Risk", "prod", "9.9.9"); err != errReleaseNotFound {
		t.Errorf("setChannel unknown version err = %v, want errReleaseNotFound", err)
	}
	// A malformed channel name is rejected.
	if err := rs.setChannel("Risk", "Prod!", "1.0.0"); err != errBadChannel {
		t.Errorf("setChannel bad name err = %v, want errBadChannel", err)
	}
}

func TestReleaseStoreValidation(t *testing.T) {
	rs, _ := newReleaseStore("")
	if _, err := rs.publish("", "1.0.0", idA, ""); err != errBadName {
		t.Errorf("empty name err = %v, want errBadName", err)
	}
	if _, err := rs.publish("X", "not-a-version", idA, ""); err != errBadVersion {
		t.Errorf("bad version err = %v, want errBadVersion", err)
	}
	if _, err := rs.publish("X", "1.0.0", "not-an-id", ""); err == nil {
		t.Errorf("bad model id err = nil, want an error")
	}
}

func TestReleaseStorePersistence(t *testing.T) {
	dir := t.TempDir()
	rs, err := newReleaseStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rs.now = fixedClock()
	if _, err := rs.publish("Pricing", "1.0.0", idA, "notes"); err != nil {
		t.Fatal(err)
	}
	if err := rs.setChannel("Pricing", "stable", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, releaseManifest)); err != nil {
		t.Fatalf("manifest not written: %v", err)
	}

	// A fresh store over the same dir recovers the catalog.
	rs2, err := newReleaseStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if mid, ok := rs2.resolve("Pricing@stable"); !ok || mid != idA {
		t.Errorf("reloaded resolve stable = %q,%v, want %q", mid, ok, idA)
	}
	mr, _ := rs2.get("Pricing")
	if len(mr.Releases) != 1 || mr.Releases[0].Notes != "notes" {
		t.Errorf("reloaded releases = %+v, want the persisted one with notes", mr.Releases)
	}
}

func TestReleaseStoreCorruptManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, releaseManifest), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, err := newReleaseStore(dir)
	if err == nil {
		t.Errorf("corrupt manifest err = nil, want an error the caller can log")
	}
	// The store is still usable (empty), not nil.
	if rs == nil || len(rs.snapshot()) != 0 {
		t.Errorf("store after corrupt manifest = %+v, want a usable empty store", rs)
	}
}
