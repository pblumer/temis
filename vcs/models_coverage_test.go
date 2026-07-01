package vcs_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/vcs"
)

// failCommitWriter commits with an error, to drive the commit-failure branch of
// Propose. Branch creation succeeds so the flow reaches the commit step.
type failCommitWriter struct{ fakeWriter }

func (w *failCommitWriter) Commit(_ context.Context, _ vcs.RepoRef, req vcs.CommitRequest) (vcs.CommitResult, error) {
	w.order = append(w.order, "commit")
	w.commit = req
	return vcs.CommitResult{}, vcs.ErrConflict
}

func TestModelsList_readerErrorPropagated(t *testing.T) {
	m := vcs.NewModels(errLister{&fakeReader{}}, nil)

	_, err := m.List(context.Background(), repo, "main", "models")
	if err == nil {
		t.Fatal("expected error from List when ListFiles fails")
	}
	if !errors.Is(err, vcs.ErrUnauthorized) {
		t.Errorf("error %v does not wrap ErrUnauthorized", err)
	}
}

func TestPropose_readOnlyModelsError(t *testing.T) {
	m := vcs.NewModels(&fakeReader{}, dmn.New())

	_, err := m.Propose(context.Background(), repo, vcs.Proposal{
		Path: "models/dish.dmn", XML: dishXML(t), Base: "main", Branch: "b", Title: "t",
	})
	if err == nil || !strings.Contains(err.Error(), "no Writer") {
		t.Errorf("want read-only error, got %v", err)
	}
}

func TestPropose_commitFailureWrapped(t *testing.T) {
	w := &failCommitWriter{}
	m := vcs.NewModelsWithWriter(notFoundLister{&fakeReader{}}, w, dmn.New())

	_, err := m.Propose(context.Background(), repo, vcs.Proposal{
		Path: "models/dish.dmn", XML: dishXML(t), Base: "main", Branch: "edit", Message: "m", Title: "t",
	})
	if err == nil || !strings.Contains(err.Error(), "commit") {
		t.Errorf("want wrapped commit error, got %v", err)
	}
	if got := strings.Join(w.order, ","); got != "branch,commit" {
		t.Errorf("must stop after failed commit; order = %q", got)
	}
}

// fileSHA returns "" when ListFiles succeeds but contains no entry matching the
// requested path (e.g. only a directory or a sibling file is returned). Propose
// then commits with an empty PrevSHA.
func TestPropose_noMatchingEntryEmptyPrevSHA(t *testing.T) {
	r := &fakeReader{
		defaultRef: "main",
		dirs: map[string][]vcs.File{
			// The path is listed but the only entry is a directory (IsDir),
			// and a sibling whose Path differs — neither matches.
			"main\x00models/dish.dmn": {
				{Path: "models/dish.dmn", Name: "dish.dmn", IsDir: true, SHA: "dir-sha"},
				{Path: "models/other.dmn", Name: "other.dmn", IsDir: false, SHA: "other-sha"},
			},
		},
	}
	w := &fakeWriter{}
	m := vcs.NewModelsWithWriter(r, w, dmn.New())

	_, err := m.Propose(context.Background(), repo, vcs.Proposal{
		Path: "models/dish.dmn", XML: dishXML(t), Base: "main", Branch: "b", Message: "m", Title: "t",
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if w.commit.PrevSHA != "" {
		t.Errorf("no matching entry must yield empty PrevSHA, got %q", w.commit.PrevSHA)
	}
}

// TestLoad_defaultRefLabel exercises refLabel's empty-ref branch via the error
// message produced when loading a missing file on the default branch.
func TestLoad_defaultRefLabelInError(t *testing.T) {
	m := vcs.NewModels(&fakeReader{defaultRef: "main"}, nil)

	_, _, err := m.Load(context.Background(), repo, "", "models/nope.dmn")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "(default branch)") {
		t.Errorf("error should name the default branch, got %v", err)
	}
}
