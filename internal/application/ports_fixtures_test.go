package application

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// The doubles here stand in for the ports, and are small because the ports are
// defined by technology: a store that only hands out bytes, a parser that only
// sees bytes. Every test that opens a session goes through them, which is why
// they live apart from any one test.

type fakeFileStore struct {
	data map[string][]byte
	meta Meta
	err  error
}

func (f fakeFileStore) Read(path string) ([]byte, Meta, error) {
	if f.err != nil {
		return nil, nil, f.err
	}

	src, ok := f.data[path]
	if !ok {
		return nil, nil, fs.ErrNotExist
	}

	return src, f.meta, nil
}

func (fakeFileStore) Write(string, []byte) error { return errors.ErrUnsupported }

func (fakeFileStore) HasChangedSince(string, Meta) (ChangeStatus, error) {
	return ChangeNone, errors.ErrUnsupported
}

// fakeMeta stands in for what a real store keeps about a file. A pointer
// makes it identifiable, which is what lets the test check that the value is
// carried through untouched.
type fakeMeta struct{}

type fakeParser struct {
	root domain.Node
	err  error

	gotSrc     []byte
	gotDialect domain.Dialect
}

func (p *fakeParser) Parse(src []byte, d domain.Dialect) (domain.Node, error) {
	p.gotSrc, p.gotDialect = src, d

	if p.err != nil {
		return nil, p.err
	}

	return p.root, nil
}

// fakeRenderer records renderer delegation. Tests of rendered content use the
// real renderers instead.
type fakeRenderer struct {
	lines []Line

	gotRoot domain.Node
	gotOpt  RenderOptions
	calls   int
}

func (r *fakeRenderer) Render(root domain.Node, opt RenderOptions) []Line {
	r.gotRoot, r.gotOpt = root, opt
	r.calls++

	return r.lines
}

// The source uses four spaces so that the detected layout cannot be confused
// with the default one.
const testSource = "{\n    \"a\": 1\n}\n"

// testTree is the document the source above parses to, as far as the tests
// that read it back are concerned.
func testTree(t *testing.T) domain.Node {
	t.Helper()

	root, err := domain.NewObject([]domain.Member{{Key: "a", Value: domain.NewNumber("1")}})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	return root
}
