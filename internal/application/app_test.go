package application

import (
	"errors"
	"testing"

	"github.com/ytakahashi/pino/internal/application/documentview"
	"github.com/ytakahashi/pino/internal/domain"
)

// Opening a document, and the frame a session hands back to be drawn.

func TestOpenLoadsAndInitialisesADocument(t *testing.T) {
	t.Parallel()

	root := testTree(t)
	meta := &fakeMeta{}
	parser := &fakeParser{root: root}
	renderer := &fakeRenderer{lines: []documentview.Line{{Kind: documentview.LineSingle}}}
	app := New(Deps{
		Parser:   parser,
		Files:    &fakeFileStore{data: map[string][]byte{"conf/app.json": []byte(testSource)}, meta: meta},
		JSONView: renderer,
		TreeView: renderer,
	}, Config{})

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

// TestOpenReplacesTheCurrentDocument covers a second document arriving in a
// session that has already been looked at: what is left of the first one must
// not describe the second.
func TestOpenReplacesTheCurrentDocument(t *testing.T) {
	t.Parallel()

	app := New(Deps{
		Parser: &fakeParser{root: testTree(t)},
		Files: &fakeFileStore{data: map[string][]byte{
			"first.json":  []byte(testSource),
			"second.json": []byte("{\n  \"a\": 1\n}\n"),
		}},
		JSONView: &fakeRenderer{},
		TreeView: &fakeRenderer{},
	}, Config{})

	if err := app.Open("first.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Stand in for a session the user has worked in: a node folded away, the
	// cursor moved into it, the view scrolled and a dialog open.
	app.view.Collapsed["/server"] = struct{}{}
	app.view.Cursor = domain.Path{}.Child(domain.KeySegment("server"))
	app.view.Scroll = 12
	app.flow = &editFlow{op: opChangeType, step: stepConfirm}

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

func TestOpenLeavesTheStateAloneOnFailure(t *testing.T) {
	t.Parallel()

	readErr := errors.New("read failed")
	parseErr := errors.New("parse failed")

	tests := map[string]struct {
		files  *fakeFileStore
		parser *fakeParser
		want   error
	}{
		"the file cannot be read": {
			files:  &fakeFileStore{err: readErr},
			parser: &fakeParser{root: domain.NewNull()},
			want:   readErr,
		},
		"the file is not JSON": {
			files:  &fakeFileStore{data: map[string][]byte{"broken.json": []byte("{")}},
			parser: &fakeParser{err: parseErr},
			want:   parseErr,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			renderer := &fakeRenderer{}
			app := New(Deps{Parser: tc.parser, Files: tc.files, JSONView: renderer, TreeView: renderer}, Config{})

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

			if frame := app.Frame(); frame.Lines != nil {
				t.Errorf("Frame().Lines = %v after a failed Open, want nil", frame.Lines)
			}

			if renderer.calls != 0 {
				t.Errorf("renderer called %d times with no document open", renderer.calls)
			}
		})
	}
}

func TestFrameReturnsTheRenderedWindow(t *testing.T) {
	t.Parallel()

	root := testTree(t)
	want := []documentview.Line{{Kind: documentview.LineOpen}, {Kind: documentview.LineClose}}
	renderer := &fakeRenderer{lines: want}
	app := New(Deps{
		Parser:   &fakeParser{root: root},
		Files:    &fakeFileStore{data: map[string][]byte{"a.json": []byte(testSource)}},
		JSONView: renderer,
		TreeView: renderer,
	}, Config{})

	if err := app.Open("a.json"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	frame := app.Frame()

	if len(frame.Lines) != len(want) {
		t.Fatalf("Frame().Lines = %v, want %v", frame.Lines, want)
	}

	// Both rows of the fake carry the root path, and the cursor starts there.
	if frame.Cursor != 0 {
		t.Errorf("Frame().Cursor = %d, want 0", frame.Cursor)
	}

	if frame.Scroll != 0 {
		t.Errorf("Frame().Scroll = %d, want 0", frame.Scroll)
	}

	// One picture costs one render; asking for the rows, the cursor row and
	// the window separately would cost three.
	if renderer.calls != 1 {
		t.Errorf("renderer called %d times for one frame, want 1", renderer.calls)
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

// A session with nothing open still has to answer, since the terminal draws
// before a document is necessarily there.
func TestFrameIsEmptyWithoutADocument(t *testing.T) {
	t.Parallel()

	renderer := &fakeRenderer{lines: []documentview.Line{{Kind: documentview.LineOpen}}}
	app := New(Deps{JSONView: renderer, TreeView: renderer}, Config{})

	frame := app.Frame()

	if frame.Lines != nil {
		t.Errorf("Frame().Lines = %v, want none", frame.Lines)
	}

	if frame.Cursor != -1 {
		t.Errorf("Frame().Cursor = %d, want -1", frame.Cursor)
	}

	if renderer.calls != 0 {
		t.Errorf("renderer called %d times with no document open", renderer.calls)
	}
}

func TestViewStateTogglesFolds(t *testing.T) {
	t.Parallel()

	view := NewViewState()
	server := path(domain.KeySegment("server"))

	if view.IsCollapsed(server) {
		t.Error("a fresh view state has something folded")
	}

	if !view.Collapse(server) {
		t.Error("Collapse() = false on a node that was open")
	}

	if !view.IsCollapsed(server) {
		t.Error("IsCollapsed() = false after folding")
	}

	// Folding twice is not a change, which is how an action knows whether the
	// rows have to be produced again.
	if view.Collapse(server) {
		t.Error("Collapse() = true on a node already folded")
	}

	// The set is what the renderer reads, keyed by the text of the path.
	if _, ok := view.RenderOptions().Collapsed["/server"]; !ok {
		t.Errorf("the renderer sees %v, want the pointer of the folded node", view.RenderOptions().Collapsed)
	}

	if !view.Expand(server) {
		t.Error("Expand() = false on a node that was folded")
	}

	if view.Expand(server) {
		t.Error("Expand() = true on a node already open")
	}

	if view.IsCollapsed(server) {
		t.Error("IsCollapsed() = true after unfolding")
	}
}

// The root is addressed by the empty pointer, which a set keyed by text can
// easily fail to hold apart from "nothing is folded".
func TestViewStateFoldsTheRoot(t *testing.T) {
	t.Parallel()

	view := NewViewState()

	if !view.Collapse(domain.Path{}) {
		t.Fatal("Collapse(root) = false")
	}

	if !view.IsCollapsed(domain.Path{}) {
		t.Error("IsCollapsed(root) = false after folding it")
	}

	if view.IsCollapsed(path(domain.KeySegment("server"))) {
		t.Error("folding the root folded a member as well")
	}
}

func TestDoAppliesEveryAction(t *testing.T) {
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

			app := New(Deps{Parser: &fakeParser{}, Files: &fakeFileStore{}, JSONView: &fakeRenderer{}, TreeView: &fakeRenderer{}}, Config{})

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

// Nothing here may depend on a document being open: the terminal reports its
// size before one necessarily is, and a key press is not refused either.
func TestActionsDoNothingWithoutADocument(t *testing.T) {
	t.Parallel()

	actions := map[string]Action{
		"next":        ActionMoveNext{},
		"prev":        ActionMovePrev{},
		"in":          ActionMoveIn{},
		"out":         ActionMoveOut{},
		"resize":      ActionResize{Height: 10},
		"toggle view": ActionToggleView{},
	}

	for name, act := range actions {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := New(Deps{JSONView: documentview.NewJSONRenderer(), TreeView: documentview.NewTreeRenderer()}, Config{})

			if effects := app.Do(act); effects != nil {
				t.Errorf("Do() = %v with no document open, want none", effects)
			}

			if frame := app.Frame(); frame.Cursor != -1 || frame.Scroll != 0 {
				t.Errorf("Frame() = %+v with no document open", frame)
			}
		})
	}
}
