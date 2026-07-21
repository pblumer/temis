package service

import (
	"errors"
	"net/http"
	"sort"
)

// HTTP surface for model releases (ADR-0037). Releases are a top-level resource
// (not nested under /v1/models/{id}) so a stable human name — not a content hash
// — keys them, and publishing carries the name in the body rather than the path.

type publishReleaseRequest struct {
	// ModelID is the content-addressed revision to tag. A release reference
	// (name@version) is also accepted here — it resolves to the concrete id first —
	// so a channel can be re-pointed by publishing an existing release under a new
	// version.
	ModelID string `json:"modelId"`
	// Name defaults to the model's display name when empty, so the modeler can
	// publish the currently open model by id alone.
	Name    string `json:"name,omitempty"`
	Version string `json:"version"`
	Notes   string `json:"notes,omitempty"`
}

type setChannelRequest struct {
	Channel string `json:"channel"`
	Version string `json:"version"`
}

type releaseDTO struct {
	Version     string `json:"version"`
	ModelID     string `json:"modelId"`
	PublishedAt string `json:"publishedAt"`
	Notes       string `json:"notes,omitempty"`
}

type modelReleasesDTO struct {
	Name     string            `json:"name"`
	Releases []releaseDTO      `json:"releases"`
	Channels map[string]string `json:"channels,omitempty"`
}

type releasesListResponse struct {
	Models []modelReleasesDTO `json:"models"`
	Count  int                `json:"count"`
}

func toReleaseDTOs(rels []release) []releaseDTO {
	out := make([]releaseDTO, 0, len(rels))
	for _, r := range rels {
		out = append(out, releaseDTO(r))
	}
	return out
}

// handlePublishRelease tags a content-addressed revision as (name, version). The
// model must be loaded; the release is immutable, so re-using a (name, version)
// is a 409.
func (s *Server) handlePublishRelease(w http.ResponseWriter, r *http.Request) {
	var req publishReleaseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	sm, ok := s.lookup(req.ModelID)
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	name := req.Name
	if name == "" {
		name = sm.name
	}
	if _, err := s.releases.publish(name, req.Version, sm.id, req.Notes); err != nil {
		writeReleaseError(w, err)
		return
	}
	mr, _ := s.releases.get(name)
	w.Header().Set("Location", "/v1/releases/"+name)
	writeJSON(w, http.StatusCreated, modelReleasesDTO{
		Name:     name,
		Releases: toReleaseDTOs(mrReleases(mr)),
		Channels: mrChannels(mr),
	})
}

// handleListReleases returns every model's releases and channels, name-sorted.
func (s *Server) handleListReleases(w http.ResponseWriter, _ *http.Request) {
	all := s.releases.snapshot()
	out := make([]modelReleasesDTO, 0, len(all))
	for name, mr := range all {
		out = append(out, modelReleasesDTO{Name: name, Releases: toReleaseDTOs(mr.Releases), Channels: mr.Channels})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, releasesListResponse{Models: out, Count: len(out)})
}

// handleGetReleases returns one model's releases and channels.
func (s *Server) handleGetReleases(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mr, ok := s.releases.get(name)
	if !ok {
		writeProblem(w, http.StatusNotFound, "RELEASE_NOT_FOUND", "no releases for that model")
		return
	}
	writeJSON(w, http.StatusOK, modelReleasesDTO{Name: name, Releases: toReleaseDTOs(mr.Releases), Channels: mr.Channels})
}

// handleSetChannel points a moving channel at an already-published version.
func (s *Server) handleSetChannel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req setChannelRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := s.releases.setChannel(name, req.Channel, req.Version); err != nil {
		writeReleaseError(w, err)
		return
	}
	mr, _ := s.releases.get(name)
	writeJSON(w, http.StatusOK, modelReleasesDTO{Name: name, Releases: toReleaseDTOs(mrReleases(mr)), Channels: mrChannels(mr)})
}

// writeReleaseError maps a release-store error to an RFC-7807 response.
func writeReleaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errReleaseExists):
		writeProblem(w, http.StatusConflict, "RELEASE_EXISTS", "a release with that version already exists")
	case errors.Is(err, errReleaseNotFound):
		writeProblem(w, http.StatusNotFound, "RELEASE_NOT_FOUND", "no such release")
	case errors.Is(err, errBadVersion):
		writeProblem(w, http.StatusBadRequest, "INVALID_VERSION", "version must be like 2.1.0, 2.1.0-rc.1 or v3")
	case errors.Is(err, errBadChannel):
		writeProblem(w, http.StatusBadRequest, "INVALID_CHANNEL", "channel must be lowercase letters, digits and dashes")
	case errors.Is(err, errBadName):
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "name must not be empty")
	default:
		writeProblem(w, http.StatusInternalServerError, "RELEASE_WRITE_FAILED", err.Error())
	}
}

func mrReleases(mr *modelReleases) []release {
	if mr == nil {
		return nil
	}
	return mr.Releases
}

func mrChannels(mr *modelReleases) map[string]string {
	if mr == nil {
		return nil
	}
	return mr.Channels
}
