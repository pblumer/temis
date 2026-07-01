package vcs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pblumer/temis/dmn"
)

// errReadOnly is returned by the write methods when no Writer was configured.
var errReadOnly = errors.New("vcs: Models has no Writer; construct it with NewModelsWithWriter to enable writes")

// dmnExt is the conventional file extension for DMN documents written by
// dmn-js and other tools.
const dmnExt = ".dmn"

// flowExt is the conventional file extension for decision-flow descriptors
// (ADR-0026, WP-94).
const flowExt = ".flow.json"

// Models loads DMN documents from a version-controlled repository and compiles
// them with an engine. It is the library entry point that ties a Reader
// (how to reach the repository) to the dmn engine (how to compile what it
// finds), so a caller can list and evaluate the models on any branch, tag or
// commit.
//
// Models is safe for concurrent use when its Reader and Engine are (both are by
// contract). It holds no per-repository state; the ref is passed on every call.
type Models struct {
	reader Reader
	writer Writer // optional; nil for a read-only Models
	engine *dmn.Engine
}

// NewModels returns a read-only Models backed by reader and engine. If engine is
// nil a default engine is used. reader must not be nil. The write methods (Save,
// Propose) are disabled; use NewModelsWithWriter to enable them.
func NewModels(reader Reader, engine *dmn.Engine) *Models {
	if engine == nil {
		engine = dmn.New()
	}
	return &Models{reader: reader, engine: engine}
}

// NewModelsWithWriter returns a Models that can also write (Save, Propose). The
// reader is used for browsing and for resolving the optimistic-concurrency base
// of an edit; the writer performs branches, commits and pull requests. If engine
// is nil a default engine is used. reader and writer must not be nil.
func NewModelsWithWriter(reader Reader, writer Writer, engine *dmn.Engine) *Models {
	if engine == nil {
		engine = dmn.New()
	}
	return &Models{reader: reader, writer: writer, engine: engine}
}

// List returns the DMN files (those whose name ends in ".dmn") directly under
// dir at ref, sorted by path. Directories and non-DMN files are omitted. An
// empty dir lists the repository root; an empty ref means the default branch.
func (m *Models) List(ctx context.Context, repo RepoRef, ref, dir string) ([]File, error) {
	entries, err := m.reader.ListFiles(ctx, repo, ref, dir)
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(strings.ToLower(e.Name), dmnExt) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ListFlows returns the decision-flow descriptor files (those whose name ends in
// ".flow.json") directly under dir at ref, sorted by path. It mirrors List for
// flows (ADR-0026, WP-94); the descriptors themselves are read with the reader's
// ReadFile and compiled by package flow, so vcs stays free of a flow dependency.
func (m *Models) ListFlows(ctx context.Context, repo RepoRef, ref, dir string) ([]File, error) {
	entries, err := m.reader.ListFiles(ctx, repo, ref, dir)
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(strings.ToLower(e.Name), flowExt) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Load reads the DMN document at path and ref and compiles it. The returned
// Definitions and Diagnostics are exactly those of dmn.Engine.Compile; a
// malformed document (or a read failure) is a hard error, while per-decision
// compile problems come back through the diagnostics.
func (m *Models) Load(ctx context.Context, repo RepoRef, ref, path string) (*dmn.Definitions, dmn.Diagnostics, error) {
	xml, err := m.reader.ReadFile(ctx, repo, ref, path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s@%s: %w", path, refLabel(ref), err)
	}
	return m.engine.Compile(ctx, xml)
}

// Save validates xml by compiling it — refusing to commit a model that does not
// even parse — and commits it to branch at path. prevSHA carries the
// optimistic-concurrency base (empty = create a new file); obtain it from a
// prior List entry's SHA or a previous CommitResult.BlobSHA. It returns
// ErrConflict if the file moved off prevSHA since, and errors if no Writer is
// configured.
func (m *Models) Save(ctx context.Context, repo RepoRef, branch, path string, xml []byte, message, prevSHA string) (CommitResult, error) {
	if m.writer == nil {
		return CommitResult{}, errReadOnly
	}
	if _, _, err := m.engine.Compile(ctx, xml); err != nil {
		return CommitResult{}, fmt.Errorf("refusing to commit malformed model %s: %w", path, err)
	}
	return m.writer.Commit(ctx, repo, CommitRequest{
		Branch:  branch,
		Path:    path,
		Content: xml,
		Message: message,
		PrevSHA: prevSHA,
	})
}

// Proposal describes an edit to land on a fresh branch and surface as a pull
// request against a base branch — the whole "edit on a branch → open a PR"
// flow in one value.
type Proposal struct {
	// Path is the DMN file to write.
	Path string
	// XML is the new file content (validated by compiling before anything is
	// written).
	XML []byte
	// Base is the branch to branch off and open the pull request against.
	Base string
	// Branch is the new branch to create and commit on.
	Branch string
	// Message is the commit message.
	Message string
	// Title is the pull-request title.
	Title string
	// Body is the pull-request description.
	Body string
}

// Propose validates p.XML, creates p.Branch off p.Base, commits the model to it
// and opens a pull request from p.Branch into p.Base. The optimistic-concurrency
// base is resolved automatically from the file's current state on p.Base (a file
// that does not yet exist is created). It errors if no Writer is configured.
//
// Merging the pull request is left to the provider; Propose only opens it.
func (m *Models) Propose(ctx context.Context, repo RepoRef, p Proposal) (PullRequest, error) {
	if m.writer == nil {
		return PullRequest{}, errReadOnly
	}
	if _, _, err := m.engine.Compile(ctx, p.XML); err != nil {
		return PullRequest{}, fmt.Errorf("refusing to propose malformed model %s: %w", p.Path, err)
	}
	prevSHA, err := m.fileSHA(ctx, repo, p.Base, p.Path)
	if err != nil {
		return PullRequest{}, fmt.Errorf("resolve %s@%s: %w", p.Path, refLabel(p.Base), err)
	}
	if _, err := m.writer.CreateBranch(ctx, repo, p.Branch, p.Base); err != nil {
		return PullRequest{}, fmt.Errorf("create branch %s: %w", p.Branch, err)
	}
	if _, err := m.writer.Commit(ctx, repo, CommitRequest{
		Branch:  p.Branch,
		Path:    p.Path,
		Content: p.XML,
		Message: p.Message,
		PrevSHA: prevSHA,
	}); err != nil {
		return PullRequest{}, fmt.Errorf("commit %s on %s: %w", p.Path, p.Branch, err)
	}
	return m.writer.OpenPullRequest(ctx, repo, PullRequestRequest{
		Title: p.Title,
		Body:  p.Body,
		Head:  p.Branch,
		Base:  p.Base,
	})
}

// fileSHA returns the blob SHA of path at ref, or "" when no such file exists
// (so a Propose of a new file creates it). It uses the reader's single-file
// listing.
func (m *Models) fileSHA(ctx context.Context, repo RepoRef, ref, path string) (string, error) {
	entries, err := m.reader.ListFiles(ctx, repo, ref, path)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	for _, e := range entries {
		if e.Path == path && !e.IsDir {
			return e.SHA, nil
		}
	}
	return "", nil
}

// refLabel renders a ref for messages, naming the default branch when ref is
// empty.
func refLabel(ref string) string {
	if ref == "" {
		return "(default branch)"
	}
	return ref
}
