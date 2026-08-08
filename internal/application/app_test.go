package application

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

// The doubles below are small because the ports are defined by technology:
// a store that only hands out bytes, a parser that only sees bytes.

type fakeFiles struct {
	data map[string][]byte
	meta Meta
	err  error
}

func (f fakeFiles) Read(path string) ([]byte, Meta, error) {
	if f.err != nil {
		return nil, nil, f.err
	}

	src, ok := f.data[path]
	if !ok {
		return nil, nil, fs.ErrNotExist
	}

	return src, f.meta, nil
}

func (fakeFiles) Write(string, []byte) error { return errors.ErrUnsupported }

func (fakeFiles) HasChangedSince(string, Meta) (ChangeStatus, error) {
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

func testTree(t *testing.T) domain.Node {
	t.Helper()

	root, err := domain.NewObject([]domain.Member{{Key: "a", Value: domain.NewNumber("1")}})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	return root
}

func TestOpen(t *testing.T) {
	t.Parallel()

	root := testTree(t)
	meta := &fakeMeta{}
	parser := &fakeParser{root: root}
	renderer := &fakeRenderer{lines: []Line{{Kind: LineSingle}}}
	app := New(Deps{
		Parser:   parser,
		Files:    fakeFiles{data: map[string][]byte{"conf/app.json": []byte(testSource)}, meta: meta},
		Renderer: renderer,
	})

	if err := app.Open("conf/app.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got := string(parser.gotSrc); got != testSource {
		t.Errorf("parser saw %q, want the bytes the store returned", got)
	}

	if parser.gotDialect != domain.StrictJSON {
		t.Errorf("parser saw dialect %+v, want StrictJSON", parser.gotDialect)
	}

	if app.doc == nil || app.doc.Root() != root {
		t.Errorf("open document holds %v, want the parsed tree", app.doc)
	}

	// The meta is carried as handed over. Reading it is the store's job.
	if app.meta != meta {
		t.Errorf("meta = %v, want the value the store returned", app.meta)
	}

	status := app.Status()

	if status.Name != "app.json" {
		t.Errorf("Status().Name = %q, want %q", status.Name, "app.json")
	}

	if status.Indent != "    " {
		t.Errorf("Status().Indent = %q, want the layout detected in the source", status.Indent)
	}

	if status.Mode != ModeNormal || status.ViewMode != ViewJSON {
		t.Errorf("Status() = %+v, want a normal-mode JSON view", status)
	}

	if status.Dirty {
		t.Error("Status().Dirty = true on a freshly opened document, want false")
	}
}

// TestOpenAgain covers a second document arriving in a session that has
// already been looked at: what is left of the first one must not describe the
// second.
func TestOpenAgain(t *testing.T) {
	t.Parallel()

	app := New(Deps{
		Parser: &fakeParser{root: testTree(t)},
		Files: fakeFiles{data: map[string][]byte{
			"first.json":  []byte(testSource),
			"second.json": []byte("{\n  \"a\": 1\n}\n"),
		}},
		Renderer: &fakeRenderer{},
	})

	if err := app.Open("first.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Stand in for a session the user has worked in: a node folded away, the
	// cursor moved into it, the view scrolled and a dialog open.
	app.view.Collapsed["/server"] = struct{}{}
	app.view.Cursor = domain.Path{}.Child(domain.KeySegment("server"))
	app.view.Scroll = 12
	app.mode = ModeConfirm

	if err := app.Open("second.json"); err != nil {
		t.Fatalf("Open again: %v", err)
	}

	if len(app.view.Collapsed) != 0 {
		t.Errorf("%d nodes still folded from the previous document, want none", len(app.view.Collapsed))
	}

	if !app.view.Cursor.IsRoot() {
		t.Errorf("cursor at %q, want the root of the new document", app.view.Cursor)
	}

	if app.view.Scroll != 0 {
		t.Errorf("scroll = %d, want 0", app.view.Scroll)
	}

	if app.Mode() != ModeNormal {
		t.Errorf("mode = %v, want %v", app.Mode(), ModeNormal)
	}

	if got, want := app.Status().Indent, "  "; got != want {
		t.Errorf("Status().Indent = %q, want the layout of the new document (%q)", got, want)
	}
}

func TestOpenFailure(t *testing.T) {
	t.Parallel()

	readErr := errors.New("read failed")
	parseErr := errors.New("parse failed")

	tests := map[string]struct {
		files  fakeFiles
		parser *fakeParser
		want   error
	}{
		"the file cannot be read": {
			files:  fakeFiles{err: readErr},
			parser: &fakeParser{root: domain.NewNull()},
			want:   readErr,
		},
		"the file is not JSON": {
			files:  fakeFiles{data: map[string][]byte{"broken.json": []byte("{")}},
			parser: &fakeParser{err: parseErr},
			want:   parseErr,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			renderer := &fakeRenderer{}
			app := New(Deps{Parser: tc.parser, Files: tc.files, Renderer: renderer})

			// The error travels out as it was raised: the command line turns
			// it into a message, and only it knows the path the user typed.
			if err := app.Open("broken.json"); !errors.Is(err, tc.want) {
				t.Fatalf("Open error = %v, want %v", err, tc.want)
			}

			// A document that could not be opened must not be half open.
			if app.doc != nil {
				t.Errorf("a document is open after a failed Open: %v", app.doc)
			}

			if app.Status().Name != "" {
				t.Errorf("Status().Name = %q after a failed Open, want empty", app.Status().Name)
			}

			if lines := app.Lines(); lines != nil {
				t.Errorf("Lines() = %v after a failed Open, want nil", lines)
			}

			if renderer.calls != 0 {
				t.Errorf("renderer called %d times with no document open", renderer.calls)
			}
		})
	}
}

func TestLines(t *testing.T) {
	t.Parallel()

	root := testTree(t)
	want := []Line{{Kind: LineOpen}, {Kind: LineClose}}
	renderer := &fakeRenderer{lines: want}
	app := New(Deps{
		Parser:   &fakeParser{root: root},
		Files:    fakeFiles{data: map[string][]byte{"a.json": []byte(testSource)}},
		Renderer: renderer,
	})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got := app.Lines(); len(got) != len(want) {
		t.Fatalf("Lines() = %v, want %v", got, want)
	}

	if renderer.gotRoot != root {
		t.Errorf("renderer saw %v, want the open tree", renderer.gotRoot)
	}

	// The view state decides what is folded; a fresh document folds nothing.
	if len(renderer.gotOpt.Collapsed) != 0 {
		t.Errorf("renderer saw %d folded nodes, want none", len(renderer.gotOpt.Collapsed))
	}

	// It also decides how much of a long value is drawn, and a document opened
	// without anyone choosing still has a limit.
	if renderer.gotOpt.MaxStrLen <= 0 {
		t.Errorf("renderer saw MaxStrLen = %d, want a limit", renderer.gotOpt.MaxStrLen)
	}
}

// otherAction is an Action this layer has no handling for.
type otherAction struct{}

func (otherAction) isAction() {}

func TestDo(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		act  Action
		want []Effect
	}{
		"quitting asks the program to stop": {
			act:  ActionQuit{},
			want: []Effect{EffectQuit{}},
		},
		"an unhandled action does nothing": {
			act:  otherAction{},
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := New(Deps{Parser: &fakeParser{}, Files: fakeFiles{}, Renderer: &fakeRenderer{}})

			got := app.Do(tc.act)
			if len(got) != len(tc.want) {
				t.Fatalf("Do(%T) = %v, want %v", tc.act, got, tc.want)
			}

			for i, effect := range got {
				if effect != tc.want[i] {
					t.Errorf("Do(%T)[%d] = %v, want %v", tc.act, i, effect, tc.want[i])
				}
			}
		})
	}
}
