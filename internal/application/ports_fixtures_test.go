package application

import (
	"io/fs"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// The doubles here stand in for the ports, and are small because the ports are
// defined by technology: a store that only hands out bytes, a parser that only
// sees bytes. Every test that opens a session goes through them, which is why
// they live apart from any one test.

// fakeFileStore is a file system of one table.
//
// What it answers is set per test rather than worked out from the table: the
// point of a save test is what the session does with each answer the port is
// allowed to give, and several of those — a rename that committed and then
// failed, a file that changed between two calls — are awkward to arrange in a
// real one and trivial to state here.
type fakeFileStore struct {
	data map[string][]byte
	meta Meta
	err  error

	// What a check before writing answers.
	status    ChangeStatus
	statusErr error

	// What a write answers.
	outcome  WriteOutcome
	writeErr error

	// What the session asked for, in order.
	reads   []string
	checks  []string
	writes  []string
	written [][]byte
}

func (f *fakeFileStore) Read(path string) ([]byte, Meta, error) {
	f.reads = append(f.reads, path)

	if f.err != nil {
		return nil, nil, f.err
	}

	src, ok := f.data[path]
	if !ok {
		return nil, nil, fs.ErrNotExist
	}

	return src, f.meta, nil
}

func (f *fakeFileStore) Write(path string, data []byte) (WriteOutcome, error) {
	f.writes = append(f.writes, path)
	f.written = append(f.written, data)

	return f.outcome, f.writeErr
}

func (f *fakeFileStore) HasChangedSince(path string, _ Meta) (ChangeStatus, error) {
	f.checks = append(f.checks, path)

	return f.status, f.statusErr
}

// fakeMeta stands in for what a real store keeps about a file. A pointer
// makes it identifiable, which is what lets the test check that the value is
// carried through untouched.
type fakeMeta struct{}

type fakeParser struct {
	root domain.Node
	err  error

	// parse, when set, answers instead of root. It is what a save test needs:
	// the bytes handed to a parser during a save are the ones just encoded,
	// and what has to come back is the tree they were encoded from.
	parse func(src []byte, d domain.Dialect) (domain.Node, error)

	gotSrc     []byte
	gotDialect domain.Dialect
	calls      int
}

func (p *fakeParser) Parse(src []byte, d domain.Dialect) (domain.Node, error) {
	p.gotSrc, p.gotDialect = src, d
	p.calls++

	if p.parse != nil {
		return p.parse(src, d)
	}

	if p.err != nil {
		return nil, p.err
	}

	return p.root, nil
}

// fakeRenderer records renderer delegation. Tests of rendered content use the
// real renderers instead.
type fakeRenderer struct {
	lines []documentview.Line

	gotRoot domain.Node
	gotOpt  documentview.Options
	calls   int
}

func (r *fakeRenderer) Render(root domain.Node, opt documentview.Options) []documentview.Line {
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
