package vcs_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/vcs"
)

// fakeWriter records the write calls made against it and lets a test force
// failures. It implements vcs.Writer.
type fakeWriter struct {
	order      []string
	branch     struct{ name, from string }
	commit     vcs.CommitRequest
	pr         vcs.PullRequestRequest
	failBranch error
}

func (w *fakeWriter) CreateBranch(_ context.Context, _ vcs.RepoRef, name, from string) (vcs.Branch, error) {
	w.order = append(w.order, "branch")
	w.branch.name, w.branch.from = name, from
	if w.failBranch != nil {
		return vcs.Branch{}, w.failBranch
	}
	return vcs.Branch{Name: name, Commit: "newbase"}, nil
}

func (w *fakeWriter) Commit(_ context.Context, _ vcs.RepoRef, req vcs.CommitRequest) (vcs.CommitResult, error) {
	w.order = append(w.order, "commit")
	w.commit = req
	return vcs.CommitResult{CommitSHA: "c1", BlobSHA: "b1"}, nil
}

func (w *fakeWriter) OpenPullRequest(_ context.Context, _ vcs.RepoRef, req vcs.PullRequestRequest) (vcs.PullRequest, error) {
	w.order = append(w.order, "pr")
	w.pr = req
	return vcs.PullRequest{Number: 7, URL: "https://example/pr/7", State: "open"}, nil
}

// errLister is a Reader whose ListFiles fails with a non-NotFound error.
type errLister struct{ *fakeReader }

func (errLister) ListFiles(context.Context, vcs.RepoRef, string, string) ([]vcs.File, error) {
	return nil, vcs.ErrUnauthorized
}

// notFoundLister is a Reader whose ListFiles reports ErrNotFound, to drive the
// "file does not yet exist" branch of Propose.
type notFoundLister struct{ *fakeReader }

func (notFoundLister) ListFiles(context.Context, vcs.RepoRef, string, string) ([]vcs.File, error) {
	return nil, vcs.ErrNotFound
}

func TestSave_refusesMalformedModel(t *testing.T) {
	w := &fakeWriter{}
	m := vcs.NewModelsWithWriter(&fakeReader{}, w, dmn.New())

	_, err := m.Save(context.Background(), repo, "main", "models/x.dmn", []byte("not xml at all"), "msg", "")
	if err == nil {
		t.Fatal("expected error for malformed model")
	}
	if len(w.order) != 0 {
		t.Errorf("nothing should be committed; got calls %v", w.order)
	}
}

func TestSave_commitsValidModel(t *testing.T) {
	w := &fakeWriter{}
	m := vcs.NewModelsWithWriter(&fakeReader{}, w, dmn.New())

	res, err := m.Save(context.Background(), repo, "main", "models/dish.dmn", dishXML(t), "update dish", "prev-blob")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if res.BlobSHA != "b1" {
		t.Errorf("result = %+v", res)
	}
	if w.commit.Branch != "main" || w.commit.Path != "models/dish.dmn" || w.commit.PrevSHA != "prev-blob" {
		t.Errorf("commit request = %+v", w.commit)
	}
}

func TestSave_readOnlyModelsError(t *testing.T) {
	m := vcs.NewModels(&fakeReader{}, dmn.New())

	_, err := m.Save(context.Background(), repo, "main", "models/dish.dmn", dishXML(t), "msg", "")
	if err == nil || !strings.Contains(err.Error(), "no Writer") {
		t.Errorf("want read-only error, got %v", err)
	}
}

func TestPropose_fullFlowInOrder(t *testing.T) {
	r := &fakeReader{
		defaultRef: "main",
		dirs: map[string][]vcs.File{
			"main\x00models/dish.dmn": {{Path: "models/dish.dmn", Name: "dish.dmn", SHA: "base-blob"}},
		},
	}
	w := &fakeWriter{}
	m := vcs.NewModelsWithWriter(r, w, dmn.New())

	pr, err := m.Propose(context.Background(), repo, vcs.Proposal{
		Path:    "models/dish.dmn",
		XML:     dishXML(t),
		Base:    "main",
		Branch:  "edit-dish",
		Message: "tweak dish",
		Title:   "Update dish",
		Body:    "why",
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if got := strings.Join(w.order, ","); got != "branch,commit,pr" {
		t.Errorf("call order = %q, want branch,commit,pr", got)
	}
	if w.branch.name != "edit-dish" || w.branch.from != "main" {
		t.Errorf("branch = %+v", w.branch)
	}
	if w.commit.Branch != "edit-dish" || w.commit.PrevSHA != "base-blob" {
		t.Errorf("commit resolved wrong base: %+v", w.commit)
	}
	if w.pr.Head != "edit-dish" || w.pr.Base != "main" {
		t.Errorf("pr = %+v", w.pr)
	}
	if pr.Number != 7 {
		t.Errorf("pr number = %d, want 7", pr.Number)
	}
}

func TestPropose_newFileHasEmptyPrevSHA(t *testing.T) {
	w := &fakeWriter{}
	m := vcs.NewModelsWithWriter(notFoundLister{&fakeReader{}}, w, dmn.New())

	_, err := m.Propose(context.Background(), repo, vcs.Proposal{
		Path: "models/new.dmn", XML: dishXML(t), Base: "main", Branch: "add-model", Message: "add", Title: "Add",
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if w.commit.PrevSHA != "" {
		t.Errorf("new file must have empty PrevSHA, got %q", w.commit.PrevSHA)
	}
}

func TestPropose_branchFailureWrapped(t *testing.T) {
	w := &fakeWriter{failBranch: vcs.ErrConflict}
	m := vcs.NewModelsWithWriter(notFoundLister{&fakeReader{}}, w, dmn.New())

	_, err := m.Propose(context.Background(), repo, vcs.Proposal{
		Path: "models/dish.dmn", XML: dishXML(t), Base: "main", Branch: "dup", Title: "t",
	})
	if err == nil || !strings.Contains(err.Error(), "create branch") {
		t.Errorf("want wrapped create-branch error, got %v", err)
	}
	if got := strings.Join(w.order, ","); got != "branch" {
		t.Errorf("must stop after failed branch; order = %q", got)
	}
}

func TestPropose_readerErrorWrapped(t *testing.T) {
	w := &fakeWriter{}
	m := vcs.NewModelsWithWriter(errLister{&fakeReader{}}, w, dmn.New())

	_, err := m.Propose(context.Background(), repo, vcs.Proposal{
		Path: "models/dish.dmn", XML: dishXML(t), Base: "main", Branch: "b", Title: "t",
	})
	if err == nil {
		t.Fatal("expected error when base SHA cannot be resolved")
	}
	if len(w.order) != 0 {
		t.Errorf("nothing should be written; got %v", w.order)
	}
}

func TestNewModelsWithWriter_nilEngineDefaults(t *testing.T) {
	w := &fakeWriter{}
	m := vcs.NewModelsWithWriter(&fakeReader{}, w, nil)

	if _, err := m.Save(context.Background(), repo, "main", "models/dish.dmn", dishXML(t), "msg", ""); err != nil {
		t.Fatalf("Save with defaulted engine: %v", err)
	}
}

func TestPropose_refusesMalformedBeforeWriting(t *testing.T) {
	w := &fakeWriter{}
	m := vcs.NewModelsWithWriter(&fakeReader{}, w, dmn.New())

	_, err := m.Propose(context.Background(), repo, vcs.Proposal{
		Path: "models/x.dmn", XML: []byte("garbage"), Base: "main", Branch: "b", Title: "t",
	})
	if err == nil {
		t.Fatal("expected error for malformed model")
	}
	if len(w.order) != 0 {
		t.Errorf("nothing should be written; got %v", w.order)
	}
}
