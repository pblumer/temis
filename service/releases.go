package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ADR-0037: model releases. Every save produces a content-addressed revision
// (a draft, modelId = sha256). A *release* is a named, immutable pointer that
// tags one such revision — (name, version) → modelId, git-tag-like, with no
// duplicated content. An optional moving *channel* (latest/stable/…) points at a
// released version. A shared resolver turns "name@version", "name@channel" or a
// bare "name" (→ its latest channel) into exactly one modelId, so every
// model-id-taking surface (evaluate, xml, graph, flow steps, MCP) accepts a
// stable release name. Only metadata is stored — the XML stays in the
// content-addressed model store (ADR-0027).

// versionPattern matches a well-formed release version: an optional "v", one to
// three dot-separated numeric components and an optional pre-release suffix. It
// accepts SemVer (2.1.0, 2.1.0-rc.1 — recommended, ADR-0019) and the simpler
// monotonic form (v1, v2). The server enforces only well-formedness and
// uniqueness; the major-vs-minor classification stays human judgement (ADR-0019).
var versionPattern = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){0,2}(-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)

// channelPattern matches a well-formed channel name (latest, stable, prod, …):
// lowercase letters, digits and dashes, so a channel never collides with a
// version and stays URL-safe in a path segment.
var channelPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// latestChannel is the channel that publish() moves to the highest version
// automatically; other channels are set manually.
const latestChannel = "latest"

var (
	// errReleaseExists is returned when publishing a (name, version) that already
	// exists — a release is immutable, so re-publishing the same coordinates is a
	// conflict (409), not an overwrite.
	errReleaseExists = errors.New("release already exists")
	// errReleaseNotFound is returned when a channel points at, or a lookup asks
	// for, a version that was never published.
	errReleaseNotFound = errors.New("release not found")
	// errBadVersion / errBadChannel / errBadName flag malformed input (400).
	errBadVersion = errors.New("malformed version")
	errBadChannel = errors.New("malformed channel")
	errBadName    = errors.New("empty name")
)

// release is one immutable publication: a version tag over a content-addressed
// revision, with the time it was cut and optional human notes.
type release struct {
	Version     string `json:"version"`
	ModelID     string `json:"modelId"`
	PublishedAt string `json:"publishedAt"`
	Notes       string `json:"notes,omitempty"`
}

// modelReleases holds every release of one named model, newest-first, plus its
// moving channels (channel → version).
type modelReleases struct {
	Releases []release         `json:"releases"`
	Channels map[string]string `json:"channels,omitempty"`
}

// releaseStore is the catalog of releases and channels, keyed by model name. It
// is safe for concurrent use. When dir is non-empty the whole catalog is
// persisted as a single atomically-written releases.json next to the model store
// (ADR-0027/0035); with dir empty it lives purely in memory (like an in-memory
// server's flows), so publishing works in tests and ephemeral servers too.
type releaseStore struct {
	mu     sync.Mutex
	dir    string
	byName map[string]*modelReleases
	now    func() time.Time // injectable clock; defaults to time.Now
}

// releaseManifest is the filename of the persisted catalog inside dir.
const releaseManifest = "releases.json"

// newReleaseStore opens the release catalog rooted at dir. An empty dir keeps it
// in-memory only (no persistence). A present-but-unreadable/corrupt manifest is
// reported as an error so the caller can log and continue with an empty catalog
// rather than silently losing releases.
func newReleaseStore(dir string) (*releaseStore, error) {
	rs := &releaseStore{dir: dir, byName: map[string]*modelReleases{}, now: time.Now}
	if dir == "" {
		return rs, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, releaseManifest))
	if err != nil {
		if os.IsNotExist(err) {
			return rs, nil // no releases yet
		}
		return rs, fmt.Errorf("release store: %w", err)
	}
	if err := json.Unmarshal(data, &rs.byName); err != nil {
		rs.byName = map[string]*modelReleases{}
		return rs, fmt.Errorf("release store: parse %s: %w", releaseManifest, err)
	}
	if rs.byName == nil {
		rs.byName = map[string]*modelReleases{}
	}
	return rs, nil
}

// persist writes the whole catalog atomically (temp file + rename, like the model
// store) when a dir is configured. It must be called with mu held. In-memory
// stores (dir == "") are a no-op.
func (rs *releaseStore) persist() error {
	if rs.dir == "" {
		return nil
	}
	data, err := json.MarshalIndent(rs.byName, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(rs.dir, ".tmp-releases-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(rs.dir, releaseManifest)); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// publish records modelID as (name, version) with optional notes and moves the
// "latest" channel to the highest version. It rejects a malformed name/version,
// a modelID that is not a content-addressed id, and a duplicate (name, version).
// The referenced model's existence is checked by the caller (the store keeps no
// model state).
func (rs *releaseStore) publish(name, version, modelID, notes string) (release, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if name == "" {
		return release{}, errBadName
	}
	if !versionPattern.MatchString(version) {
		return release{}, errBadVersion
	}
	if !validModelID(modelID) {
		return release{}, fmt.Errorf("%w: %q", errBadVersion, modelID)
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()
	mr := rs.byName[name]
	if mr == nil {
		mr = &modelReleases{Channels: map[string]string{}}
		rs.byName[name] = mr
	}
	for _, r := range mr.Releases {
		if r.Version == version {
			return release{}, errReleaseExists
		}
	}
	rel := release{Version: version, ModelID: modelID, PublishedAt: rs.now().UTC().Format(time.RFC3339), Notes: notes}
	mr.Releases = append(mr.Releases, rel)
	sort.SliceStable(mr.Releases, func(i, j int) bool {
		return compareVersions(mr.Releases[i].Version, mr.Releases[j].Version) > 0 // newest (highest) first
	})
	if mr.Channels == nil {
		mr.Channels = map[string]string{}
	}
	mr.Channels[latestChannel] = mr.Releases[0].Version
	if err := rs.persist(); err != nil {
		return rel, err
	}
	return rel, nil
}

// setChannel points a moving channel (e.g. "stable") at an already-published
// version. It rejects a malformed channel and an unknown version.
func (rs *releaseStore) setChannel(name, channel, version string) error {
	name = strings.TrimSpace(name)
	channel = strings.TrimSpace(channel)
	version = strings.TrimSpace(version)
	if name == "" {
		return errBadName
	}
	if !channelPattern.MatchString(channel) {
		return errBadChannel
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	mr := rs.byName[name]
	if mr == nil {
		return errReleaseNotFound
	}
	found := false
	for _, r := range mr.Releases {
		if r.Version == version {
			found = true
			break
		}
	}
	if !found {
		return errReleaseNotFound
	}
	if mr.Channels == nil {
		mr.Channels = map[string]string{}
	}
	mr.Channels[channel] = version
	return rs.persist()
}

// resolve turns a release reference into a concrete modelId. It accepts
// "name@version", "name@channel" and a bare "name" (→ its latest channel, or the
// highest version if no channel is set). It returns ok=false for a raw model id
// (the caller handles those directly) and for any unknown reference, so it can be
// called speculatively on every path id.
func (rs *releaseStore) resolve(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || validModelID(ref) {
		return "", false
	}
	name, sel := ref, latestChannel
	if i := strings.LastIndex(ref, "@"); i >= 0 {
		name, sel = ref[:i], ref[i+1:]
	}
	name = strings.TrimSpace(name)
	sel = strings.TrimSpace(sel)
	if name == "" || sel == "" {
		return "", false
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()
	mr := rs.byName[name]
	if mr == nil || len(mr.Releases) == 0 {
		return "", false
	}
	// A channel wins over a same-named version (channels are lowercase letters/
	// digits; versions start with a digit or 'v', so real overlap is rare).
	if v, ok := mr.Channels[sel]; ok {
		sel = v
	}
	for _, r := range mr.Releases {
		if r.Version == sel {
			return r.ModelID, true
		}
	}
	return "", false
}

// get returns a copy of one model's releases and channels, if any exist.
func (rs *releaseStore) get(name string) (*modelReleases, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	mr, ok := rs.byName[strings.TrimSpace(name)]
	if !ok {
		return nil, false
	}
	return cloneModelReleases(mr), true
}

// snapshot returns a copy of the whole catalog, keyed by name.
func (rs *releaseStore) snapshot() map[string]*modelReleases {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make(map[string]*modelReleases, len(rs.byName))
	for name, mr := range rs.byName {
		out[name] = cloneModelReleases(mr)
	}
	return out
}

// releasedIDs returns the set of every model id any release points at, so a GC
// pass can tell a released revision from a prunable draft (ADR-0037, WP-143).
func (rs *releaseStore) releasedIDs() map[string]bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := map[string]bool{}
	for _, mr := range rs.byName {
		for _, r := range mr.Releases {
			out[r.ModelID] = true
		}
	}
	return out
}

func cloneModelReleases(mr *modelReleases) *modelReleases {
	cp := &modelReleases{Releases: append([]release(nil), mr.Releases...)}
	if len(mr.Channels) > 0 {
		cp.Channels = make(map[string]string, len(mr.Channels))
		for k, v := range mr.Channels {
			cp.Channels[k] = v
		}
	}
	return cp
}

// compareVersions orders two well-formed versions: numeric component by component
// (a missing component counts as 0), then a version without a pre-release ranks
// above one with the same core but a pre-release (2.1.0 > 2.1.0-rc.1), and two
// pre-releases compare lexically. A leading "v" is ignored. It returns -1, 0 or 1.
func compareVersions(a, b string) int {
	ca, pa := splitVersion(a)
	cb, pb := splitVersion(b)
	for i := 0; i < 3; i++ {
		na, nb := coreComponent(ca, i), coreComponent(cb, i)
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	// Equal cores: a pre-release ranks below the plain release.
	switch {
	case pa == "" && pb == "":
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	case pa < pb:
		return -1
	case pa > pb:
		return 1
	default:
		return 0
	}
}

// splitVersion strips a leading "v" and splits into the numeric core parts and
// the pre-release suffix (empty when absent).
func splitVersion(v string) ([]string, string) {
	v = strings.TrimPrefix(v, "v")
	core, pre := v, ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		core, pre = v[:i], v[i+1:]
	}
	return strings.Split(core, "."), pre
}

// coreComponent returns the i-th numeric core component as an int, or 0 when the
// component is missing or non-numeric (the version was validated on the way in).
func coreComponent(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return n
}
