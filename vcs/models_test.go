package vcs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/vcs"
)

// fakeReader is an in-memory vcs.Reader for testing the Models binding without a
// network. Files are keyed by "ref\x00path"; an empty ref maps to defaultRef.
type fakeReader struct {
	defaultRef string
	branches   []vcs.Branch
	commits    map[string][]vcs.Commit // ref -> commits
	dirs       map[string][]vcs.File   // "ref\x00dir" -> entries
	files      map[string][]byte       // "ref\x00path" -> content
}

func (f *fakeReader) ref(ref string) string {
	if ref == "" {
		return f.defaultRef
	}
	return ref
}

func (f *fakeReader) ListBranches(_ context.Context, _ vcs.RepoRef) ([]vcs.Branch, error) {
	return f.branches, nil
}

func (f *fakeReader) ListCommits(_ context.Context, _ vcs.RepoRef, ref string) ([]vcs.Commit, error) {
	return f.commits[f.ref(ref)], nil
}

func (f *fakeReader) ListFiles(_ context.Context, _ vcs.RepoRef, ref, dir string) ([]vcs.File, error) {
	return f.dirs[f.ref(ref)+"\x00"+dir], nil
}

func (f *fakeReader) ReadFile(_ context.Context, _ vcs.RepoRef, ref, path string) ([]byte, error) {
	b, ok := f.files[f.ref(ref)+"\x00"+path]
	if !ok {
		return nil, vcs.ErrNotFound
	}
	return b, nil
}

func dishXML(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "dmn", "testdata", "models", "dish_15.dmn"))
	if err != nil {
		t.Fatalf("read dish model: %v", err)
	}
	return b
}

var repo = vcs.RepoRef{Owner: "pblumer", Name: "temis"}

func TestModelsList_filtersToDMN(t *testing.T) {
	r := &fakeReader{
		defaultRef: "main",
		dirs: map[string][]vcs.File{
			"main\x00models": {
				{Path: "models/dish.dmn", Name: "dish.dmn", IsDir: false},
				{Path: "models/readme.md", Name: "readme.md", IsDir: false},
				{Path: "models/sub", Name: "sub", IsDir: true},
				{Path: "models/loan.DMN", Name: "loan.DMN", IsDir: false}, // case-insensitive
			},
		},
	}
	m := vcs.NewModels(r, nil)

	got, err := m.List(context.Background(), repo, "", "models")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"models/dish.dmn", "models/loan.DMN"}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(got), len(want), got)
	}
	for i, f := range got {
		if f.Path != want[i] {
			t.Errorf("file[%d] = %q, want %q", i, f.Path, want[i])
		}
	}
}

func TestModelsLoad_compilesAndEvaluates(t *testing.T) {
	r := &fakeReader{
		defaultRef: "main",
		files: map[string][]byte{
			"release\x00models/dish.dmn": dishXML(t),
		},
	}
	m := vcs.NewModels(r, dmn.New())

	defs, diags, err := m.Load(context.Background(), repo, "release", "models/dish.dmn")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range diags {
		if d.Severity == dmn.SevError {
			t.Fatalf("unexpected compile error diagnostic: %+v", d)
		}
	}

	dec, err := defs.Decision("Dish")
	if err != nil {
		t.Fatalf("Decision: %v", err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Season": "Winter", "Guest Count": 8})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := res.Outputs["Dish"]; got != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", got)
	}
}

func TestModelsLoad_missingFileWrapsNotFound(t *testing.T) {
	m := vcs.NewModels(&fakeReader{defaultRef: "main"}, nil)

	_, _, err := m.Load(context.Background(), repo, "main", "models/nope.dmn")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, vcs.ErrNotFound) {
		t.Errorf("error %v does not wrap ErrNotFound", err)
	}
}
